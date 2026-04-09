// Package main provides the CLI entry point for argus.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/nextzhou/argus/internal/doctor"
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
	var jsonFlag bool

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

			results := doctor.RunAllChecks(projectRoot)
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
	return cmd
}

func writeDoctorReport(out io.Writer, results []doctor.CheckResult) int {
	summary := summarizeDoctorResults(results)
	for _, result := range results {
		switch result.Status {
		case "pass":
			_, _ = fmt.Fprintf(out, "[PASS] %s\n", result.Name)
		case "fail":
			_, _ = fmt.Fprintf(out, "[FAIL] %s: %s\n", result.Name, result.Message)
			if result.Suggestion != "" {
				_, _ = fmt.Fprintf(out, "  → %s\n", result.Suggestion)
			}
		case "skip":
			_, _ = fmt.Fprintf(out, "[SKIP] %s: %s\n", result.Name, result.Message)
		}
	}

	_, _ = fmt.Fprintf(out, "\n%d checks: %d passed, %d failed, %d skipped\n", summary.Total, summary.Passed, summary.Failed, summary.Skipped)

	return summary.Failed
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
