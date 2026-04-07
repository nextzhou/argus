package main

import (
	"fmt"
	"os"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/spf13/cobra"
)

func newWorkflowInspectCmd() *cobra.Command {
	var markdownFlag bool

	cmd := &cobra.Command{
		Use:   "inspect [dir]",
		Short: "Inspect workflow definitions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := ".argus/workflows"
			if len(args) > 0 {
				dir = args[0]
			}

			report, err := workflow.InspectDirectory(dir)
			if err != nil {
				errBytes, _ := core.ErrorEnvelope(err.Error())
				_, _ = os.Stdout.Write(errBytes)
				_, _ = os.Stdout.WriteString("\n")
				return fmt.Errorf("workflow inspect failed: %w", err)
			}

			if markdownFlag {
				renderWorkflowMarkdown(cmd, report)
				return nil
			}

			outBytes, err := core.OKEnvelope(report)
			if err != nil {
				return fmt.Errorf("marshaling output: %w", err)
			}
			_, _ = os.Stdout.Write(outBytes)
			_, _ = os.Stdout.WriteString("\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&markdownFlag, "markdown", false, "Output human-readable markdown summary")
	return cmd
}

func renderWorkflowMarkdown(cmd *cobra.Command, report *workflow.InspectReport) {
	w := cmd.OutOrStdout()
	if report.Valid {
		_, _ = w.Write([]byte("# Workflow Inspect\n\nAll workflows valid.\n"))
		return
	}

	_, _ = w.Write([]byte("# Workflow Inspect\n\nValidation errors found:\n\n"))
	for filename, fr := range report.Files {
		if !fr.Valid {
			for _, e := range fr.Errors {
				_, _ = fmt.Fprintf(w, "- %s (%s): %s\n", filename, e.Path, e.Message)
			}
		}
	}
}
