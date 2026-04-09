package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/invariant"
	"github.com/spf13/cobra"
)

type checkStepOutput struct {
	Description string `json:"description"`
	Status      string `json:"status"`
	Output      string `json:"output,omitempty"`
}

type checkResultOutput struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Steps       []checkStepOutput `json:"steps"`
	// Workflow and Prompt use *string so they serialize as null (not absent) in JSON.
	// They are only populated when the invariant check fails, providing remediation info.
	// For passing invariants these remain nil → JSON null.
	Workflow *string `json:"workflow"`
	Prompt   *string `json:"prompt"`
}

type invariantCheckOutput struct {
	Passed  int                 `json:"passed"`
	Failed  int                 `json:"failed"`
	Results []checkResultOutput `json:"results"`
}

func newInvariantCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check [id]",
		Short: "Run invariant checks",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			invariantsDir := filepath.Join(".argus", "invariants")

			if len(args) == 1 {
				return runSingleCheck(cmd.Context(), args[0], invariantsDir)
			}
			return runAllChecks(cmd.Context(), invariantsDir)
		},
	}
}

func runSingleCheck(ctx context.Context, id, invariantsDir string) error {
	filePath := filepath.Join(invariantsDir, id+".yaml")
	inv, err := invariant.ParseInvariantFile(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			errBytes, _ := core.ErrorEnvelope("invariant not found")
			_, _ = os.Stdout.Write(errBytes)
			_, _ = os.Stdout.WriteString("\n")
			return fmt.Errorf("invariant check failed: invariant %q not found", id)
		}
		errBytes, _ := core.ErrorEnvelope(err.Error())
		_, _ = os.Stdout.Write(errBytes)
		_, _ = os.Stdout.WriteString("\n")
		return fmt.Errorf("invariant check failed: %w", err)
	}

	result := invariant.RunCheck(ctx, inv, ".")
	output := buildCheckOutput(inv, result)

	return writeCheckOutput([]checkResultOutput{output})
}

func runAllChecks(ctx context.Context, invariantsDir string) error {
	entries, err := os.ReadDir(invariantsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return writeCheckOutput(nil)
		}
		return fmt.Errorf("reading invariants directory: %w", err)
	}

	var results []checkResultOutput
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		fullPath := filepath.Join(invariantsDir, entry.Name())
		inv, parseErr := invariant.ParseInvariantFile(fullPath)
		if parseErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Argus warning: skipping %s: %s\n", entry.Name(), parseErr)
			continue
		}

		result := invariant.RunCheck(ctx, inv, ".")
		results = append(results, buildCheckOutput(inv, result))
	}

	slices.SortFunc(results, func(a, b checkResultOutput) int {
		return strings.Compare(a.ID, b.ID)
	})

	return writeCheckOutput(results)
}

func buildCheckOutput(inv *invariant.Invariant, result *invariant.CheckResult) checkResultOutput {
	steps := make([]checkStepOutput, 0, len(result.Steps))
	for _, s := range result.Steps {
		steps = append(steps, checkStepOutput{
			Description: s.Description,
			Status:      s.Status,
			Output:      s.Output,
		})
	}

	status := "passed"
	if !result.Passed {
		status = "failed"
	}

	out := checkResultOutput{
		ID:          inv.ID,
		Description: invariantDescription(inv),
		Status:      status,
		Steps:       steps,
	}

	if !result.Passed {
		if inv.Workflow != "" {
			out.Workflow = &inv.Workflow
		}
		if inv.Prompt != "" {
			out.Prompt = &inv.Prompt
		}
	}

	return out
}

func invariantDescription(inv *invariant.Invariant) string {
	if inv.Description != "" {
		return inv.Description
	}
	shells := make([]string, 0, len(inv.Check))
	for _, step := range inv.Check {
		shells = append(shells, step.Shell)
	}
	return strings.Join(shells, "; ")
}

func writeCheckOutput(results []checkResultOutput) error {
	if results == nil {
		results = []checkResultOutput{}
	}

	passed := 0
	failed := 0
	for _, r := range results {
		if r.Status == "passed" {
			passed++
		} else {
			failed++
		}
	}

	outBytes, err := core.OKEnvelope(invariantCheckOutput{
		Passed:  passed,
		Failed:  failed,
		Results: results,
	})
	if err != nil {
		return fmt.Errorf("marshaling output: %w", err)
	}
	_, _ = os.Stdout.Write(outBytes)
	_, _ = os.Stdout.WriteString("\n")
	return nil
}
