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

	stepFailureExit     = "exit"
	stepFailureTimeout  = "timeout"
	stepFailureCanceled = "canceled"
	stepFailureExec     = "exec"

	stepTimeout = 5 * time.Second
	// SlowCheckThreshold is the aggregate automatic-check runtime above which
	// Argus surfaces a slow-check warning or diagnostic finding.
	SlowCheckThreshold = 2 * time.Second
	outputCap          = 8 * 1024
)

type checkRuntime struct {
	now         func() time.Time
	stepTimeout time.Duration
	slowCheckAt time.Duration
	runStep     func(ctx context.Context, script string, projectRoot string) stepExecutionResult
}

type stepExecutionResult struct {
	output      string
	status      string
	exitCode    *int
	failureKind string
}

// StepResult records the outcome of a single invariant shell check step.
type StepResult struct {
	Check       CheckStep
	Status      string
	Output      string
	ExitCode    *int
	FailureKind string
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
	return runCheckWithRuntime(ctx, inv, projectRoot, checkRuntime{})
}

func runCheckWithRuntime(ctx context.Context, inv *Invariant, projectRoot string, runtime checkRuntime) *CheckResult {
	if runtime.now == nil {
		runtime.now = time.Now
	}
	if runtime.stepTimeout == 0 {
		runtime.stepTimeout = stepTimeout
	}
	if runtime.slowCheckAt == 0 {
		runtime.slowCheckAt = SlowCheckThreshold
	}
	if runtime.runStep == nil {
		runtime.runStep = func(ctx context.Context, script string, projectRoot string) stepExecutionResult {
			return runStep(ctx, script, projectRoot, runtime.stepTimeout)
		}
	}

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

	startedAt := runtime.now()
	skipRemaining := false

	for _, step := range inv.Check {
		stepResult := StepResult{
			Check: step,
		}

		if skipRemaining || ctx.Err() != nil {
			stepResult.Status = stepStatusSkip
			result.Steps = append(result.Steps, stepResult)
			continue
		}

		stepCtx, cancel := context.WithTimeout(ctx, runtime.stepTimeout)
		stepStartedAt := runtime.now()
		execution := runtime.runStep(stepCtx, step.Shell, absProjectRoot)
		stepResult.Duration = runtime.now().Sub(stepStartedAt)
		cancel()

		stepResult.Status = execution.status
		stepResult.Output = execution.output
		stepResult.ExitCode = execution.exitCode
		stepResult.FailureKind = execution.failureKind
		result.Steps = append(result.Steps, stepResult)

		if execution.status == stepStatusFail {
			result.Passed = false
			skipRemaining = true
		}
	}

	result.TotalTime = runtime.now().Sub(startedAt)
	result.SlowCheck = result.TotalTime > runtime.slowCheckAt
	return result
}

func runStep(ctx context.Context, script string, projectRoot string, timeout time.Duration) stepExecutionResult {
	//nolint:gosec // Argus intentionally executes user-authored invariant shell checks; this is the product contract.
	cmd := exec.CommandContext(ctx, "/usr/bin/env", "bash", "-c", script)
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), "ARGUS_PROJECT_ROOT="+projectRoot)

	var output limitedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	if err == nil {
		return stepExecutionResult{status: stepStatusPass}
	}

	return buildFailureResult(ctx, output.String(), err, timeout)
}

func buildFailureResult(ctx context.Context, output string, err error, timeout time.Duration) stepExecutionResult {
	trimmedOutput := strings.TrimSpace(output)
	result := stepExecutionResult{status: stepStatusFail}

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.failureKind = stepFailureTimeout
		result.output = appendDiagnostic(trimmedOutput, fmt.Sprintf("command timeout after %s", timeout))
	case errors.Is(ctx.Err(), context.Canceled):
		result.failureKind = stepFailureCanceled
		result.output = appendDiagnostic(trimmedOutput, fmt.Sprintf("command canceled: %v", ctx.Err()))
	default:
		if exitCode, ok := extractExitCode(err); ok {
			result.failureKind = stepFailureExit
			result.exitCode = &exitCode
			result.output = trimOutput(trimmedOutput)
		} else {
			result.failureKind = stepFailureExec
			result.output = appendDiagnostic(trimmedOutput, err.Error())
		}
	}

	return result
}

func extractExitCode(err error) (int, bool) {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0, false
	}

	exitCode := exitErr.ExitCode()
	if exitCode < 0 {
		return 0, false
	}

	return exitCode, true
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
			return 0, fmt.Errorf("writing invariant output buffer: %w", err)
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return b.buffer.String()
}
