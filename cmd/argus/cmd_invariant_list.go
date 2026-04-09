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
	return &cobra.Command{
		Use:   "list",
		Short: "List available invariants",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			invariantsDir := filepath.Join(".argus", "invariants")

			entries, err := os.ReadDir(invariantsDir)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return writeInvariantListOutput(nil)
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

			return writeInvariantListOutput(result)
		},
	}
}

func writeInvariantListOutput(invariants []invariantListEntry) error {
	if invariants == nil {
		invariants = []invariantListEntry{}
	}
	outBytes, err := core.OKEnvelope(invariantListOutput{Invariants: invariants})
	if err != nil {
		return fmt.Errorf("marshaling output: %w", err)
	}
	_, _ = os.Stdout.Write(outBytes)
	_, _ = os.Stdout.WriteString("\n")
	return nil
}
