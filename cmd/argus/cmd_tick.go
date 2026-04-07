package main

import (
	"fmt"
	"os"

	"github.com/nextzhou/argus/internal/hook"
	"github.com/spf13/cobra"
)

// SEQUENCE-TEST: cmd_tick_lifecycle_test.go
func newTickCmd() *cobra.Command {
	var agentFlag string

	cmd := &cobra.Command{
		Use:    "tick",
		Short:  "Inject workflow context into AI agent sessions",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				_, _ = os.Stdout.WriteString("[Argus] Warning: could not determine working directory\n")
				return nil
			}

			global, _ := cmd.Flags().GetBool("global")
			if err := hook.HandleTick(agentFlag, global, cmd.InOrStdin(), os.Stdout, cwd); err != nil {
				_, _ = fmt.Fprintf(os.Stdout, "[Argus] Warning: internal error: %v\n", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&agentFlag, "agent", "", "AI agent type (claude-code, codex, opencode)")
	_ = cmd.MarkFlagRequired("agent")
	cmd.Flags().Bool("global", false, "Run in global mode (stub, M7)")

	return cmd
}
