package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeTrapCmd runs the trap command and captures stdout output.
// Tests using this helper must NOT call t.Parallel since os.Stdout is redirected.
func executeTrapCmd(t *testing.T, stdinJSON string, args ...string) ([]byte, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := newTrapCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	if stdinJSON != "" {
		cmd.SetIn(strings.NewReader(stdinJSON))
	}
	cmdErr := cmd.Execute()

	require.NoError(t, w.Close())
	os.Stdout = old

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return out, cmdErr
}

func TestTrapWithAgentAndStdin(t *testing.T) {
	out, err := executeTrapCmd(t, "{}", "--agent", "claude-code")
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(out, &result))

	hookOutput, ok := result["hookSpecificOutput"].(map[string]any)
	require.True(t, ok, "hookSpecificOutput should be a map")

	assert.Equal(t, "PreToolUse", hookOutput["hookEventName"])
	assert.Equal(t, "allow", hookOutput["permissionDecision"])
}

func TestTrapWithEmptyStdin(t *testing.T) {
	out, err := executeTrapCmd(t, "", "--agent", "claude-code")
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(out, &result))

	hookOutput, ok := result["hookSpecificOutput"].(map[string]any)
	require.True(t, ok, "hookSpecificOutput should be a map")

	assert.Equal(t, "PreToolUse", hookOutput["hookEventName"])
	assert.Equal(t, "allow", hookOutput["permissionDecision"])
}

func TestTrapWithDifferentAgents(t *testing.T) {
	agents := []string{"claude-code", "codex", "opencode"}

	for _, agent := range agents {
		t.Run(agent, func(t *testing.T) {
			out, err := executeTrapCmd(t, "{}", "--agent", agent)
			require.NoError(t, err)

			var result map[string]any
			require.NoError(t, json.Unmarshal(out, &result))

			hookOutput, ok := result["hookSpecificOutput"].(map[string]any)
			require.True(t, ok, "hookSpecificOutput should be a map")

			assert.Equal(t, "PreToolUse", hookOutput["hookEventName"])
			assert.Equal(t, "allow", hookOutput["permissionDecision"])
		})
	}
}

func TestTrapWithoutAgentFlag(t *testing.T) {
	_, err := executeTrapCmd(t, "{}")
	require.Error(t, err, "should error when --agent flag is missing")
}
