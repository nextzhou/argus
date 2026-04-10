package main

import (
	"io"

	"github.com/nextzhou/argus/internal/core"
	"github.com/spf13/cobra"
)

// SEQUENCE-TEST: cmd_tick_lifecycle_test.go
func newTrapCmd() *cobra.Command {
	var agentFlag string

	cmd := &cobra.Command{
		Use:    "trap",
		Short:  "Reserved tool-use hook entry point (not wired in Phase 1)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Read stdin (may be empty) but don't process in Phase 1
			_, _ = io.ReadAll(cmd.InOrStdin())

			// Codex PreToolUse currently rejects permissionDecision:allow and
			// permissionDecision:ask. In Phase 1 allow-paths must therefore stay
			// silent for Codex, while Claude Code and OpenCode keep the explicit
			// allow JSON for symmetry with future deny outputs.
			if agentFlag == "codex" {
				return nil
			}

			// Phase 1: always allow
			output := map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName":      "PreToolUse",
					"permissionDecision": "allow",
				},
			}

			core.WriteJSON(cmd.OutOrStdout(), output)
			return nil
		},
	}

	cmd.Flags().StringVar(&agentFlag, "agent", "", "AI agent type (claude-code, codex, opencode)")
	_ = cmd.MarkFlagRequired("agent")
	cmd.Flags().Bool("global", false, "Run in global mode (stub, M7)")

	return cmd
}
