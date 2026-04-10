package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/session"
	"github.com/nextzhou/argus/internal/sessiontest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeTickCmd(t *testing.T, store session.Store, stdinJSON string, args ...string) ([]byte, error) {
	t.Helper()

	var out bytes.Buffer

	cmd := newTickCmdWithSessionStore(store)
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

func assertHookSafeTickText(t *testing.T, output string) {
	t.Helper()

	trimmed := strings.TrimLeft(output, " \t\r\n")
	require.NotEmpty(t, trimmed)
	assert.NotEqual(t, '[', rune(trimmed[0]))
	assert.NotEqual(t, '{', rune(trimmed[0]))
}

func TestTickNoActivePipeline(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickCommandWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)
	sessionID := sessiontest.NewSessionID(t, "tick-cli-no-pipeline")

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Chdir(oldCwd)) }()
	require.NoError(t, os.Chdir(projectRoot))

	output, cmdErr := executeTickCmd(t, sessiontest.NewMemoryStore(), mustJSONInput(t, map[string]string{
		"session_id": sessionID,
		"cwd":        projectRoot,
	}), "--agent", "claude-code")
	require.NoError(t, cmdErr)
	assertHookSafeTickText(t, string(output))
	assert.Contains(t, string(output), "Argus:")
	assert.Contains(t, string(output), "argus workflow start")
}

func TestTickSubAgentSkip(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".argus", "workflows"), 0o700))
	sessionID := sessiontest.NewSessionID(t, "tick-cli-sub-agent")

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Chdir(oldCwd)) }()
	require.NoError(t, os.Chdir(projectRoot))

	output, cmdErr := executeTickCmd(t, sessiontest.NewMemoryStore(), mustJSONInput(t, map[string]string{
		"session_id": sessionID,
		"agent_id":   "worker-1",
	}), "--agent", "claude-code")
	require.NoError(t, cmdErr)
	assert.Empty(t, string(output))
}

func TestTickFailOpen(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".argus", "workflows"), 0o700))

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Chdir(oldCwd)) }()
	require.NoError(t, os.Chdir(projectRoot))

	output, cmdErr := executeTickCmd(t, sessiontest.NewMemoryStore(), `{invalid json}`, "--agent", "claude-code")
	require.NoError(t, cmdErr)
	assertHookSafeTickText(t, string(output))
	assert.Contains(t, string(output), "Argus warning")
	assert.Contains(t, string(output), "could not parse hook input")
}

func TestTickWithoutAgent(t *testing.T) {
	sessionID := sessiontest.NewSessionID(t, "tick-cli-missing-agent")

	output, cmdErr := executeTickCmd(t, sessiontest.NewMemoryStore(), mustJSONInput(t, map[string]string{
		"session_id": sessionID,
	}))
	require.Error(t, cmdErr)
	assert.Empty(t, string(output))
	assert.Contains(t, cmdErr.Error(), "required flag(s) \"agent\" not set")
}

func writeTickCommandWorkflowFixture(t *testing.T, projectRoot, workflowID, yamlContent string) {
	t.Helper()
	workflowsDir := filepath.Join(projectRoot, ".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, workflowID+".yaml"), []byte(yamlContent), 0o600))
}
