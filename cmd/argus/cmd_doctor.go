// Package main provides the CLI entry point for argus.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/nextzhou/argus/internal/doctor"
	"github.com/nextzhou/argus/internal/scope"
	"github.com/nextzhou/argus/internal/workspace"
	"github.com/spf13/cobra"
)

type doctorSummaryOutput struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type doctorOutput struct {
	Summary doctorSummaryOutput  `json:"summary"`
	Checks  []doctor.CheckResult `json:"checks"`
}

func newDoctorCmd() *cobra.Command {
	var (
		jsonFlag        bool
		checkInvariants bool
	)

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostic checks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("getting current directory: %w", err)
			}

			projectRoot := ""
			pr, err := workspace.FindProjectRoot(cwd)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("finding project root: %w", err)
			}
			if pr != nil {
				projectRoot = pr.Path
			}

			currentScope, err := scope.ResolveScope(cwd)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("resolving scope: %w", err)
			}

			results := doctor.RunAllChecks(projectRoot, currentScope, doctor.RunOptions{
				CheckInvariants: checkInvariants,
			})
			summary := summarizeDoctorResults(results)
			if jsonFlag {
				if err := writeJSONOK(cmd, doctorOutput{Summary: summary, Checks: results}); err != nil {
					return err
				}
			} else {
				writeDoctorReport(cmd.OutOrStdout(), results)
			}

			if summary.Failed > 0 {
				return fmt.Errorf("doctor found %d failures", summary.Failed)
			}

			return nil
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	cmd.Flags().BoolVar(
		&checkInvariants,
		"check-invariants",
		false,
		"Run invariant shell checks for deep diagnostics (executes shell defined by Argus and the project)",
	)
	return cmd
}

func writeDoctorReport(out io.Writer, results []doctor.CheckResult) int {
	summary := summarizeDoctorResults(results)
	for _, result := range results {
		switch result.Status {
		case "pass":
			_, _ = fmt.Fprintf(out, "[PASS] %s\n", result.Name)
			writeDoctorDetail(out, result)
		case "fail":
			_, _ = fmt.Fprintf(out, "[FAIL] %s: %s\n", result.Name, result.Message)
			if result.Suggestion != "" {
				_, _ = fmt.Fprintf(out, "  → %s\n", result.Suggestion)
			}
			writeDoctorDetail(out, result)
		case "skip":
			_, _ = fmt.Fprintf(out, "[SKIP] %s: %s\n", result.Name, result.Message)
			if result.Suggestion != "" {
				_, _ = fmt.Fprintf(out, "  → %s\n", result.Suggestion)
			}
		}
	}

	_, _ = fmt.Fprintf(out, "\n%d checks: %d passed, %d failed, %d skipped\n", summary.Total, summary.Passed, summary.Failed, summary.Skipped)

	return summary.Failed
}

func writeDoctorDetail(out io.Writer, result doctor.CheckResult) {
	if result.Detail == nil || result.Detail.AutomaticInvariantDiagnostics == nil {
		return
	}

	diag := result.Detail.AutomaticInvariantDiagnostics
	if !diag.Enabled || len(diag.Invariants) == 0 {
		return
	}

	_, _ = fmt.Fprintf(
		out,
		"  total %.1fs (threshold %.1fs)\n",
		float64(diag.TotalTimeMS)/1000,
		float64(diag.ThresholdMS)/1000,
	)
	for _, inv := range diag.Invariants {
		_, _ = fmt.Fprintf(out, "  - %s (%s): %.1fs\n", inv.ID, inv.Auto, float64(inv.TotalTimeMS)/1000)
		for index, step := range inv.Steps {
			label := step.Description
			if label == "" {
				label = fmt.Sprintf("step %d", index+1)
			}
			_, _ = fmt.Fprintf(out, "    %d. [%s] %s - %.1fs\n", index+1, step.Status, label, float64(step.DurationMS)/1000)
		}
	}
}

func summarizeDoctorResults(results []doctor.CheckResult) doctorSummaryOutput {
	var passed, failed, skipped int
	for _, result := range results {
		switch result.Status {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}

	return doctorSummaryOutput{
		Total:   len(results),
		Passed:  passed,
		Failed:  failed,
		Skipped: skipped,
	}
}
