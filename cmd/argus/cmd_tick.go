package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/nextzhou/argus/internal/hook"
	"github.com/nextzhou/argus/internal/session"
	"github.com/spf13/cobra"
)

// SEQUENCE-TEST: cmd_tick_lifecycle_test.go
// SEQUENCE-TEST: cmd_workspace_lifecycle_test.go
func newTickCmd() *cobra.Command {
	return newTickCmdWithSessionStore(session.NewFileStore("/tmp/argus"))
}

func newTickCmdWithSessionStore(store session.Store) *cobra.Command {
	var agentFlag string
	var mockFlag bool
	var mockSessionID string

	cmd := &cobra.Command{
		Use:    "tick [--agent <name>] [--mock]",
		Short:  "Inject workflow context into AI agent sessions",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()
			cwd, err := os.Getwd()
			if err != nil {
				_, _ = w.Write([]byte("Argus warning: could not determine working directory\n"))
				return nil
			}

			global, _ := cmd.Flags().GetBool("global")
			if mockSessionID != "" && !mockFlag {
				return fmt.Errorf("--mock-session-id requires --mock")
			}

			if !mockFlag {
				if agentFlag == "" {
					return fmt.Errorf("--agent is required unless --mock is set")
				}
				if err := hook.HandleTickWithSessionStore(cmd.Context(), agentFlag, global, cmd.InOrStdin(), w, cwd, store); err != nil {
					_, _ = fmt.Fprintf(w, "Argus warning: internal error: %v\n", err)
				}
				return nil
			}

			sessionID := mockSessionID
			printGeneratedSessionID := false
			if sessionID == "" {
				sessionID, err = newMockSessionID()
				if err != nil {
					_, _ = fmt.Fprintf(w, "Argus warning: internal error: generating mock session id: %v\n", err)
					return nil
				}
				printGeneratedSessionID = true
			}

			input := &hook.AgentInput{
				SessionID: sessionID,
				CWD:       cwd,
			}

			var tickOutput bytes.Buffer
			if err := hook.HandleTickInputWithSessionStore(cmd.Context(), global, input, &tickOutput, cwd, store); err != nil {
				_, _ = fmt.Fprintf(&tickOutput, "Argus warning: internal error: %v\n", err)
			}

			if printGeneratedSessionID {
				_, _ = fmt.Fprintf(w, "Argus: Mock session: %s\n", sessionID)
			}
			if _, err := w.Write(tickOutput.Bytes()); err != nil {
				return fmt.Errorf("writing tick output: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&agentFlag, "agent", "", "AI agent type for real hook stdin parsing (claude-code, codex, opencode); optional with --mock")
	cmd.Flags().BoolVar(&mockFlag, "mock", false, "Generate mock tick input instead of reading stdin")
	cmd.Flags().StringVar(&mockSessionID, "mock-session-id", "", "Use this session ID for mock tick input")
	cmd.Flags().Bool("global", false, "Run in global mode")

	return cmd
}

func newMockSessionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}

	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(raw[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		encoded[0:8],
		encoded[8:12],
		encoded[12:16],
		encoded[16:20],
		encoded[20:32],
	), nil
}
