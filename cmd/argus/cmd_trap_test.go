package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeTrapCmd runs the trap command and captures stdout output.
func executeTrapCmd(t *testing.T, stdinJSON string, args ...string) ([]byte, error) {
	t.Helper()

	var out bytes.Buffer

	cmd := newTrapCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	if stdinJSON != "" {
		cmd.SetIn(strings.NewReader(stdinJSON))
	}
	cmdErr := cmd.Execute()

	return out.Bytes(), cmdErr
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
	agents := []string{"claude-code", "opencode"}

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

func TestTrapCodexAllowReturnsEmptyOutput(t *testing.T) {
	out, err := executeTrapCmd(t, "{}", "--agent", "codex")
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(out)))
}

func TestTrapWithoutAgentFlag(t *testing.T) {
	_, err := executeTrapCmd(t, "{}")
	require.Error(t, err, "should error when --agent flag is missing")
}
