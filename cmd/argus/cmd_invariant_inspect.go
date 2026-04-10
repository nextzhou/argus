package main

import (
	"fmt"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/spf13/cobra"
)

func newInvariantInspectCmd() *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "inspect [dir]",
		Short: "Inspect invariant definitions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := ".argus/invariants"
			if len(args) > 0 {
				dir = args[0]
			}

			workflowChecker := buildWorkflowChecker(".argus/workflows")
			allowReservedID, err := builtinInvariantAllowReservedID()
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("invariant inspect failed: %w", err)
			}

			report, err := invariant.InspectDirectory(dir, workflowChecker, allowReservedID)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("invariant inspect failed: %w", err)
			}

			if jsonFlag {
				return writeJSONOK(cmd, report)
			}

			renderInvariantInspectText(cmd, report)
			return nil
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func renderInvariantInspectText(cmd *cobra.Command, report *invariant.InspectReport) {
	w := cmd.OutOrStdout()
	if report.Valid {
		_, _ = w.Write([]byte("# Invariant Inspect\n\nAll invariants valid.\n"))
		return
	}

	_, _ = w.Write([]byte("# Invariant Inspect\n\nValidation errors found:\n\n"))
	for filename, fr := range report.Files {
		if !fr.Valid {
			for _, e := range fr.Errors {
				_, _ = fmt.Fprintf(w, "- %s: %s\n", filename, e.Message)
			}
		}
	}
}
