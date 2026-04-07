package main

import (
	"io"
	"os"

	"github.com/nextzhou/argus/internal/core"
	"github.com/spf13/cobra"
)

// SEQUENCE-TEST: cmd_tick_lifecycle_test.go
func newTrapCmd() *cobra.Command {
	var agentFlag string

	cmd := &cobra.Command{
		Use:    "trap",
		Short:  "Gate tool-use requests from AI agents (Phase 1: always allow)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Read stdin (may be empty) but don't process in Phase 1
			_, _ = io.ReadAll(cmd.InOrStdin())

			// Phase 1: always allow
			output := map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName":      "PreToolUse",
					"permissionDecision": "allow",
				},
			}

			core.WriteJSON(os.Stdout, output)
			return nil
		},
	}

	cmd.Flags().StringVar(&agentFlag, "agent", "", "AI agent type (claude-code, codex, opencode)")
	_ = cmd.MarkFlagRequired("agent")
	cmd.Flags().Bool("global", false, "Run in global mode (stub, M7)")

	return cmd
}
