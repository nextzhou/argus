package main

import (
	"fmt"
	"os"

	"github.com/nextzhou/argus/internal/artifact"
	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/workspace"
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

			projectRoot := currentInvariantInspectProjectRoot()
			workflowChecker := buildWorkflowChecker(projectRoot, ".argus/workflows")
			allowReservedID, err := builtinInvariantAllowReservedID()
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("invariant inspect failed: %w", err)
			}

			report, err := artifact.NewInvariantProvider(projectRoot, dir).Inspect(workflowChecker, allowReservedID)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("invariant inspect failed: %w", err)
			}

			if jsonFlag {
				return writeJSONOK(cmd, report)
			}

			renderInvariantInspectText(cmd, projectRoot, report)
			return nil
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func renderInvariantInspectText(cmd *cobra.Command, projectRoot string, report *invariant.InspectReport) {
	w := cmd.OutOrStdout()
	if report.Valid {
		_, _ = w.Write([]byte("# Invariant Inspect\n\nAll invariants valid.\n"))
		return
	}

	_, _ = w.Write([]byte("# Invariant Inspect\n\nValidation errors found:\n\n"))
	for _, entry := range report.Entries {
		if entry.Valid {
			continue
		}
		sourceText := core.FormatSourceRef(projectRoot, entry.Source)
		for _, finding := range entry.Findings {
			if finding.FieldPath != "" {
				_, _ = fmt.Fprintf(w, "- %s (%s): %s\n", sourceText, finding.FieldPath, finding.Message)
				continue
			}
			_, _ = fmt.Fprintf(w, "- %s: %s\n", sourceText, finding.Message)
		}
	}
}

func currentInvariantInspectProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root, err := workspace.FindProjectRoot(cwd)
	if err != nil || root == nil {
		return ""
	}
	return root.Path
}
