package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/scope"
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
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "check [id]",
		Short: "Run invariant checks",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			s, err := scope.ResolveScope(cwd)
			if err != nil {
				return fmt.Errorf("resolving scope: %w", err)
			}
			if s == nil {
				return fmt.Errorf("not inside an Argus project or registered workspace")
			}

			if len(args) == 1 {
				return runSingleCheck(cmd, jsonFlag, args[0], s)
			}
			return runAllChecks(cmd, jsonFlag, s)
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func runSingleCheck(cmd *cobra.Command, jsonOutput bool, id string, s scope.Scope) error {
	invariants, err := s.LoadInvariants()
	if err != nil {
		writeCommandError(cmd, jsonOutput, err.Error())
		return fmt.Errorf("invariant check failed: %w", err)
	}

	var inv *invariant.Invariant
	for _, candidate := range invariants {
		if candidate.ID == id {
			inv = candidate
			break
		}
	}
	if inv == nil {
		writeCommandError(cmd, jsonOutput, "invariant not found")
		return fmt.Errorf("invariant check failed: invariant %q not found", id)
	}

	result := invariant.RunCheck(cmd.Context(), inv, s.ProjectRoot())
	output := buildCheckOutput(inv, result)

	return writeCheckOutput(cmd, jsonOutput, []checkResultOutput{output})
}

func runAllChecks(cmd *cobra.Command, jsonOutput bool, s scope.Scope) error {
	invariants, err := s.LoadInvariants()
	if err != nil {
		return fmt.Errorf("loading invariants: %w", err)
	}

	results := make([]checkResultOutput, 0, len(invariants))
	for _, inv := range invariants {
		result := invariant.RunCheck(cmd.Context(), inv, s.ProjectRoot())
		results = append(results, buildCheckOutput(inv, result))
	}

	slices.SortFunc(results, func(a, b checkResultOutput) int {
		return strings.Compare(a.ID, b.ID)
	})

	return writeCheckOutput(cmd, jsonOutput, results)
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

func writeCheckOutput(cmd *cobra.Command, jsonOutput bool, results []checkResultOutput) error {
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

	out := invariantCheckOutput{
		Passed:  passed,
		Failed:  failed,
		Results: results,
	}

	if jsonOutput {
		return writeJSONOK(cmd, out)
	}

	renderInvariantCheckText(cmd.OutOrStdout(), out)
	return nil
}

func renderInvariantCheckText(w io.Writer, out invariantCheckOutput) {
	_, _ = fmt.Fprintln(w, "Argus: Invariant check")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Summary: %d passed, %d failed\n", out.Passed, out.Failed)

	if len(out.Results) == 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "No invariants found.")
		return
	}

	if out.Failed == 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "All %d invariants passed.\n", out.Passed)
		return
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Failed invariants:")
	for _, result := range out.Results {
		if result.Status != "failed" {
			continue
		}

		_, _ = fmt.Fprintf(w, "- %s: %s\n", result.ID, result.Description)
		for _, step := range result.Steps {
			_, _ = fmt.Fprintf(w, "  Step [%s]: %s\n", step.Status, step.Description)
			if step.Output != "" {
				_, _ = fmt.Fprintf(w, "  Output: %s\n", step.Output)
			}
		}
		if result.Workflow != nil {
			_, _ = fmt.Fprintf(w, "  Workflow: %s\n", *result.Workflow)
		}
		if result.Prompt != nil {
			_, _ = fmt.Fprintf(w, "  Prompt: %s\n", *result.Prompt)
		}
	}
}
