package main

import (
	"fmt"

	"github.com/nextzhou/argus/internal/workflow"
	"github.com/spf13/cobra"
)

func newWorkflowInspectCmd() *cobra.Command {
	var jsonFlag bool

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
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("workflow inspect failed: %w", err)
			}

			if jsonFlag {
				return writeJSONOK(cmd, report)
			}

			renderWorkflowText(cmd, report)
			return nil
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func renderWorkflowText(cmd *cobra.Command, report *workflow.InspectReport) {
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
