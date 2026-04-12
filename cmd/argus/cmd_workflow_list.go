package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/nextzhou/argus/internal/scope"
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
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			s, err := scope.ResolveScope(cwd)
			if err != nil {
				return fmt.Errorf("resolving scope: %w", err)
			}
			if s == nil {
				return fmt.Errorf("not inside an Argus project or registered workspace")
			}

			summaries, err := s.Artifacts().Workflows().Summaries()
			if err != nil {
				return fmt.Errorf("loading workflow summaries: %w", err)
			}

			result := make([]workflowListEntry, 0, len(summaries))
			for _, ws := range summaries {
				result = append(result, workflowListEntry{
					ID:          ws.ID,
					Description: ws.Description,
					Jobs:        ws.Jobs,
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
