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

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/spf13/cobra"
)

type invariantListEntry struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Auto        string `json:"auto"`
	Checks      int    `json:"checks"`
}

type invariantListOutput struct {
	Invariants []invariantListEntry `json:"invariants"`
}

func newInvariantListCmd() *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available invariants",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			invariantsDir := filepath.Join(".argus", "invariants")

			entries, err := os.ReadDir(invariantsDir)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return writeInvariantListOutput(cmd, nil, jsonFlag)
				}
				return fmt.Errorf("reading invariants directory: %w", err)
			}

			var result []invariantListEntry
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
					continue
				}

				fullPath := filepath.Join(invariantsDir, entry.Name())
				inv, parseErr := invariant.ParseInvariantFile(fullPath)
				if parseErr != nil {
					_, _ = fmt.Fprintf(os.Stderr, "Argus warning: skipping %s: %s\n", entry.Name(), parseErr)
					continue
				}

				result = append(result, invariantListEntry{
					ID:          inv.ID,
					Description: invariantDescription(inv),
					Auto:        inv.Auto,
					Checks:      len(inv.Check),
				})
			}

			slices.SortFunc(result, func(a, b invariantListEntry) int {
				return strings.Compare(a.ID, b.ID)
			})

			return writeInvariantListOutput(cmd, result, jsonFlag)
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func writeInvariantListOutput(cmd *cobra.Command, invariants []invariantListEntry, jsonOutput bool) error {
	if invariants == nil {
		invariants = []invariantListEntry{}
	}

	if jsonOutput {
		return writeJSONOK(cmd, invariantListOutput{Invariants: invariants})
	}

	renderInvariantListText(cmd.OutOrStdout(), invariants)
	return nil
}

func renderInvariantListText(w io.Writer, invariants []invariantListEntry) {
	_, _ = fmt.Fprintln(w, "Argus: Invariants")
	_, _ = fmt.Fprintln(w)

	if len(invariants) == 0 {
		_, _ = fmt.Fprintln(w, "No invariants found.")
		return
	}

	for _, inv := range invariants {
		_, _ = fmt.Fprintf(w, "- %s", inv.ID)
		if inv.Description != "" {
			_, _ = fmt.Fprintf(w, " — %s", inv.Description)
		}
		_, _ = fmt.Fprintf(w, " (auto: %s, checks: %d)\n", inv.Auto, inv.Checks)
	}
}
