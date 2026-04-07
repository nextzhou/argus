package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nextzhou/argus/internal/core"
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
	return &cobra.Command{
		Use:   "list",
		Short: "List available workflows",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			workflowsDir := filepath.Join(".argus", "workflows")

			entries, err := os.ReadDir(workflowsDir)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return writeListOutput(nil)
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
					_, _ = fmt.Fprintf(os.Stderr, "[Argus] Warning: skipping %s: %s\n", name, parseErr)
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

			return writeListOutput(result)
		},
	}
}

func writeListOutput(workflows []workflowListEntry) error {
	if workflows == nil {
		workflows = []workflowListEntry{}
	}
	outBytes, err := core.OKEnvelope(workflowListOutput{Workflows: workflows})
	if err != nil {
		return fmt.Errorf("marshaling output: %w", err)
	}
	_, _ = os.Stdout.Write(outBytes)
	_, _ = os.Stdout.WriteString("\n")
	return nil
}
