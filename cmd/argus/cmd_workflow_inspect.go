package main

import (
	"fmt"
	"os"

	"github.com/nextzhou/argus/internal/artifact"
	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/nextzhou/argus/internal/workspace"
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

			projectRoot := currentProjectRoot()

			allowReservedID, err := builtinWorkflowAllowReservedID()
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("workflow inspect failed: %w", err)
			}

			report, err := artifact.NewWorkflowProvider(projectRoot, dir).Inspect(allowReservedID)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("workflow inspect failed: %w", err)
			}

			if jsonFlag {
				return writeJSONOK(cmd, report)
			}

			renderWorkflowText(cmd, projectRoot, report)
			return nil
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func renderWorkflowText(cmd *cobra.Command, projectRoot string, report *workflow.InspectReport) {
	w := cmd.OutOrStdout()
	if report.Valid {
		_, _ = w.Write([]byte("# Workflow Inspect\n\nAll workflows valid.\n"))
		return
	}

	_, _ = w.Write([]byte("# Workflow Inspect\n\nValidation errors found:\n\n"))
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

func currentProjectRoot() string {
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
