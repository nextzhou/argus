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

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostic checks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}

			projectRoot := ""
			pr, err := workspace.FindProjectRoot(cwd)
			if err != nil {
				return fmt.Errorf("finding project root: %w", err)
			}
			if pr != nil {
				projectRoot = pr.Path
			}

			results := doctor.RunAllChecks(projectRoot)
			failed := writeDoctorReport(cmd.OutOrStdout(), results)

			if failed > 0 {
				return fmt.Errorf("doctor found %d failures", failed)
			}

			return nil
		},
	}

	return cmd
}

func writeDoctorReport(out io.Writer, results []doctor.CheckResult) int {
	var passed, failed, skipped int
	for _, result := range results {
		switch result.Status {
		case "pass":
			passed++
			_, _ = fmt.Fprintf(out, "[PASS] %s\n", result.Name)
		case "fail":
			failed++
			_, _ = fmt.Fprintf(out, "[FAIL] %s: %s\n", result.Name, result.Message)
			if result.Suggestion != "" {
				_, _ = fmt.Fprintf(out, "  → %s\n", result.Suggestion)
			}
		case "skip":
			skipped++
			_, _ = fmt.Fprintf(out, "[SKIP] %s: %s\n", result.Name, result.Message)
		}
	}

	total := len(results)
	_, _ = fmt.Fprintf(out, "\n%d checks: %d passed, %d failed, %d skipped\n", total, passed, failed, skipped)

	return failed
}
