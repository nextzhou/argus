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

			invariants, err := s.LoadInvariants()
			if err != nil {
				return fmt.Errorf("loading invariants: %w", err)
			}

			result := make([]invariantListEntry, 0, len(invariants))
			for _, inv := range invariants {
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
