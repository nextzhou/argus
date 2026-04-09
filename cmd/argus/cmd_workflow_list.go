package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nextzhou/argus/internal/workflow"
	"github.com/spf13/cobra"
)

type workflowListEntry struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Jobs        int    `json:"jobs"`
}

type workflowListOutput struct {
	Workflows []workflowListEntry `json:"workflows"`
}

func newWorkflowListCmd() *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available workflows",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workflowsDir := filepath.Join(".argus", "workflows")

			entries, err := os.ReadDir(workflowsDir)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return writeListOutput(cmd, nil, jsonFlag)
				}
				return fmt.Errorf("reading workflows directory: %w", err)
			}

			var result []workflowListEntry
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
					continue
				}
				if name == "_shared.yaml" || name == "_shared.yml" {
					continue
				}

				fullPath := filepath.Join(workflowsDir, name)
				w, parseErr := workflow.ParseWorkflowFile(fullPath)
				if parseErr != nil {
					_, _ = fmt.Fprintf(os.Stderr, "Argus warning: skipping %s: %s\n", name, parseErr)
					continue
				}

				result = append(result, workflowListEntry{
					ID:          w.ID,
					Description: w.Description,
					Jobs:        len(w.Jobs),
				})
			}

			slices.SortFunc(result, func(a, b workflowListEntry) int {
				return strings.Compare(a.ID, b.ID)
			})

			return writeListOutput(cmd, result, jsonFlag)
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func writeListOutput(cmd *cobra.Command, workflows []workflowListEntry, jsonOutput bool) error {
	if workflows == nil {
		workflows = []workflowListEntry{}
	}

	if jsonOutput {
		return writeJSONOK(cmd, workflowListOutput{Workflows: workflows})
	}

	renderWorkflowListText(cmd.OutOrStdout(), workflows)
	return nil
}

func renderWorkflowListText(w io.Writer, workflows []workflowListEntry) {
	_, _ = fmt.Fprintln(w, "Argus: Workflows")
	_, _ = fmt.Fprintln(w)

	if len(workflows) == 0 {
		_, _ = fmt.Fprintln(w, "No workflows found.")
		return
	}

	for _, workflow := range workflows {
		_, _ = fmt.Fprintf(w, "- %s", workflow.ID)
		if workflow.Description != "" {
			_, _ = fmt.Fprintf(w, " — %s", workflow.Description)
		}
		_, _ = fmt.Fprintf(w, " (%d jobs)\n", workflow.Jobs)
	}
}
