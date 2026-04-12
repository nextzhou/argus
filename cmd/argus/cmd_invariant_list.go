package main

import (
	"fmt"
	"io"
	"os"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/scope"
	"github.com/spf13/cobra"
)

type invariantListEntry struct {
	ID          string `json:"id"`
	Order       int    `json:"order"`
	Description string `json:"description"`
	Auto        string `json:"auto"`
	Checks      int    `json:"checks"`
}

type invariantListOutput struct {
	Invariants        []invariantListEntry `json:"invariants"`
	InvalidInvariants []invariant.Issue    `json:"invalid_invariants"`
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

			catalog, err := s.LoadInvariantCatalog()
			if err != nil {
				return fmt.Errorf("loading invariant catalog: %w", err)
			}
			if catalog == nil {
				catalog = invariant.EmptyCatalog()
			}

			result := make([]invariantListEntry, 0, len(catalog.Invariants))
			for _, inv := range catalog.Invariants {
				result = append(result, invariantListEntry{
					ID:          inv.ID,
					Order:       inv.Order,
					Description: invariantDescription(inv),
					Auto:        inv.Auto,
					Checks:      len(inv.Check),
				})
			}

			return writeInvariantListOutput(cmd, result, catalog.Issues, jsonFlag)
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func writeInvariantListOutput(cmd *cobra.Command, invariants []invariantListEntry, invalidInvariants []invariant.Issue, jsonOutput bool) error {
	if invariants == nil {
		invariants = []invariantListEntry{}
	}
	if invalidInvariants == nil {
		invalidInvariants = []invariant.Issue{}
	}

	if jsonOutput {
		return writeJSONOK(cmd, invariantListOutput{
			Invariants:        invariants,
			InvalidInvariants: invalidInvariants,
		})
	}

	renderInvariantListText(cmd.OutOrStdout(), invariants, invalidInvariants)
	return nil
}

func renderInvariantListText(w io.Writer, invariants []invariantListEntry, invalidInvariants []invariant.Issue) {
	_, _ = fmt.Fprintln(w, "Argus: Invariants")
	_, _ = fmt.Fprintln(w)

	if len(invariants) == 0 {
		_, _ = fmt.Fprintln(w, "No invariants found.")
	} else {
		for _, inv := range invariants {
			_, _ = fmt.Fprintf(w, "- #%d %s", inv.Order, inv.ID)
			if inv.Description != "" {
				_, _ = fmt.Fprintf(w, " — %s", inv.Description)
			}
			_, _ = fmt.Fprintf(w, " (auto: %s, checks: %d)\n", inv.Auto, inv.Checks)
		}
	}

	if len(invalidInvariants) == 0 {
		return
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Invalid invariants: %d\n", len(invalidInvariants))
	for _, issue := range invalidInvariants {
		_, _ = fmt.Fprintf(w, "- %s\n", issue.String())
	}
}
