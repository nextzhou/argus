package invariant

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	stepStatusPass = "pass"
	stepStatusFail = "fail"
	stepStatusSkip = "skip"

	stepTimeout = 5 * time.Second
	slowCheckAt = 2 * time.Second
	outputCap   = 8 * 1024
)

// StepResult records the outcome of a single invariant shell check step.
type StepResult struct {
	Description string
	Status      string
	Output      string
	Duration    time.Duration
}

// CheckResult records the aggregate outcome of an invariant shell check run.
type CheckResult struct {
	InvariantID string
	Passed      bool
	Steps       []StepResult
	TotalTime   time.Duration
	SlowCheck   bool
}

// RunCheck executes invariant check steps sequentially using bash.
func RunCheck(ctx context.Context, inv *Invariant, projectRoot string) *CheckResult {
	result := &CheckResult{Passed: true}
	if inv == nil {
		return result
	}

	result.InvariantID = inv.ID
	result.Steps = make([]StepResult, 0, len(inv.Check))

	absProjectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		absProjectRoot = projectRoot
	}

	startedAt := time.Now()
	skipRemaining := false

	for _, step := range inv.Check {
		stepResult := StepResult{Description: step.Description}

		if skipRemaining || ctx.Err() != nil {
			stepResult.Status = stepStatusSkip
			result.Steps = append(result.Steps, stepResult)
			continue
		}

		stepCtx, cancel := context.WithTimeout(ctx, stepTimeout)
		stepStartedAt := time.Now()
		output, status := runStep(stepCtx, step.Shell, absProjectRoot)
		stepResult.Duration = time.Since(stepStartedAt)
		cancel()

		stepResult.Status = status
		stepResult.Output = output
		result.Steps = append(result.Steps, stepResult)

		if status == stepStatusFail {
			result.Passed = false
			skipRemaining = true
		}
	}

	result.TotalTime = time.Since(startedAt)
	result.SlowCheck = result.TotalTime > slowCheckAt
	return result
}

func runStep(ctx context.Context, script string, projectRoot string) (string, string) {
	cmd := exec.CommandContext(ctx, "/usr/bin/env", "bash", "-c", script)
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), "ARGUS_PROJECT_ROOT="+projectRoot)

	var output limitedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	if err == nil {
		return "", stepStatusPass
	}

	return buildFailureOutput(ctx, output.String(), err), stepStatusFail
}

func buildFailureOutput(ctx context.Context, output string, err error) string {
	trimmedOutput := strings.TrimSpace(output)

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return appendDiagnostic(trimmedOutput, fmt.Sprintf("command timeout after %s", stepTimeout))
	case errors.Is(ctx.Err(), context.Canceled):
		return appendDiagnostic(trimmedOutput, fmt.Sprintf("command canceled: %v", ctx.Err()))
	default:
		return appendDiagnostic(trimmedOutput, err.Error())
	}
}

func appendDiagnostic(output string, diagnostic string) string {
	if len(output) > outputCap {
		output = output[:outputCap]
	}
	if output == "" {
		return trimOutput(diagnostic)
	}
	if diagnostic == "" {
		return trimOutput(output)
	}
	reserved := len(diagnostic) + 1
	if reserved >= outputCap {
		return trimOutput(diagnostic)
	}
	if len(output) > outputCap-reserved {
		output = output[:outputCap-reserved]
	}
	return output + "\n" + diagnostic
}

func trimOutput(output string) string {
	if len(output) <= outputCap {
		return output
	}
	return output[:outputCap]
}

type limitedBuffer struct {
	buffer bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := outputCap - b.buffer.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		if _, err := b.buffer.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return b.buffer.String()
}
