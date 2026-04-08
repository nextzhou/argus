// Package main provides the CLI entry point for argus.
package main

import (
	"fmt"
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

			var passed, failed, skipped int
			for _, result := range results {
				switch result.Status {
				case "pass":
					passed++
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[PASS] %s\n", result.Name)
				case "fail":
					failed++
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[FAIL] %s: %s\n", result.Name, result.Message)
					if result.Suggestion != "" {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  → %s\n", result.Suggestion)
					}
				case "skip":
					skipped++
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[SKIP] %s: %s\n", result.Name, result.Message)
				}
			}

			total := len(results)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n%d checks: %d passed, %d failed, %d skipped\n", total, passed, failed, skipped)

			if failed > 0 {
				return fmt.Errorf("doctor found %d failures", failed)
			}

			return nil
		},
	}

	return cmd
}
