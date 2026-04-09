package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/workflow"
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

			report, err := invariant.InspectDirectory(dir, workflowChecker)
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

// buildWorkflowChecker scans workflowsDir for .yaml files and returns a
// checker function that reports whether a workflow ID exists.
func buildWorkflowChecker(workflowsDir string) func(id string) bool {
	knownIDs := make(map[string]bool)
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		// If dir doesn't exist or can't read, return empty checker (all IDs unknown)
		return func(_ string) bool { return false }
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		if entry.Name() == "_shared.yaml" {
			continue
		}
		path := filepath.Join(workflowsDir, entry.Name())
		wf, parseErr := workflow.ParseWorkflowFile(path)
		if parseErr != nil {
			continue
		}
		knownIDs[wf.ID] = true
	}

	return func(id string) bool { return knownIDs[id] }
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
