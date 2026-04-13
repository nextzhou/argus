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

func extractMockSessionID(t *testing.T, output string) string {
	t.Helper()

	line, _, _ := strings.Cut(output, "\n")
	const prefix = "Argus: Mock session: "
	require.True(t, strings.HasPrefix(line, prefix), "first line should expose generated mock session: %q", line)
	return strings.TrimPrefix(line, prefix)
}

func TestTickNoActivePipeline(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickCommandWorkflowFixture(t, projectRoot)
	sessionID := sessiontest.NewSessionID(t, "tick-cli-no-pipeline")

	t.Chdir(projectRoot)

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

	t.Chdir(projectRoot)

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

	t.Chdir(projectRoot)

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
	assert.Contains(t, cmdErr.Error(), "--agent is required unless --mock is set")
}

func TestTickMock_GeneratesVisibleSessionID(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickCommandWorkflowFixture(t, projectRoot)
	t.Chdir(projectRoot)

	output, cmdErr := executeTickCmd(t, sessiontest.NewMemoryStore(), "", "--agent", "claude-code", "--mock")
	require.NoError(t, cmdErr)

	sessionID := extractMockSessionID(t, string(output))
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, sessionID)
	assert.Contains(t, string(output), "No active pipeline")
}

func TestTickMock_WithoutAgentSucceeds(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickCommandWorkflowFixture(t, projectRoot)
	t.Chdir(projectRoot)

	output, cmdErr := executeTickCmd(t, sessiontest.NewMemoryStore(), "", "--mock")
	require.NoError(t, cmdErr)

	sessionID := extractMockSessionID(t, string(output))
	assert.NotEmpty(t, sessionID)
	assert.Contains(t, string(output), "No active pipeline")
}

func TestTickMock_WithExplicitSessionIDDoesNotPrintExtraLine(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickCommandWorkflowFixture(t, projectRoot)
	nestedDir := filepath.Join(projectRoot, "nested", "dir")
	require.NoError(t, os.MkdirAll(nestedDir, 0o700))
	t.Chdir(nestedDir)

	output, cmdErr := executeTickCmd(t, sessiontest.NewMemoryStore(), "", "--agent", "claude-code", "--mock", "--mock-session-id", "fixed-session")
	require.NoError(t, cmdErr)

	assert.NotContains(t, string(output), "Argus: Mock session:")
	assert.Contains(t, string(output), "No active pipeline")
}

func TestTickMock_WithOrWithoutAgentProducesSameBody(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickCommandWorkflowFixture(t, projectRoot)
	store := sessiontest.NewMemoryStore()
	t.Chdir(projectRoot)

	withoutAgent, err := executeTickCmd(t, store, "", "--mock", "--mock-session-id", "fixed-session")
	require.NoError(t, err)

	withAgent, err := executeTickCmd(t, store, "", "--agent", "claude-code", "--mock", "--mock-session-id", "other-fixed-session")
	require.NoError(t, err)

	assert.Equal(t, string(withoutAgent), string(withAgent))
}

func TestTickMock_GeneratesDistinctSessionIDs(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickCommandWorkflowFixture(t, projectRoot)
	store := sessiontest.NewMemoryStore()
	t.Chdir(projectRoot)

	firstOutput, err := executeTickCmd(t, store, "", "--agent", "claude-code", "--mock")
	require.NoError(t, err)
	secondOutput, err := executeTickCmd(t, store, "", "--agent", "claude-code", "--mock")
	require.NoError(t, err)

	firstID := extractMockSessionID(t, string(firstOutput))
	secondID := extractMockSessionID(t, string(secondOutput))
	assert.NotEqual(t, firstID, secondID)
}

func TestTickMock_ExplicitSessionIDReusesState(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickCommandWorkflowFixture(t, projectRoot)
	store := sessiontest.NewMemoryStore()
	t.Chdir(projectRoot)

	_, err := executeStartCmd(t, "release")
	require.NoError(t, err)

	firstOutput, err := executeTickCmd(t, store, "", "--agent", "claude-code", "--mock", "--mock-session-id", "fixed-session")
	require.NoError(t, err)
	assert.Contains(t, string(firstOutput), "Current Job:")
	assert.Contains(t, string(firstOutput), "Run tests")

	secondOutput, err := executeTickCmd(t, store, "", "--agent", "claude-code", "--mock", "--mock-session-id", "fixed-session")
	require.NoError(t, err)
	assert.Contains(t, string(secondOutput), "run_tests")
	assert.Contains(t, string(secondOutput), "argus job-done")
	assert.NotContains(t, string(secondOutput), "Current Job:")
	assert.NotContains(t, string(secondOutput), "Run tests")
}

func TestTickMockSessionIDRequiresMock(t *testing.T) {
	output, cmdErr := executeTickCmd(t, sessiontest.NewMemoryStore(), "", "--agent", "claude-code", "--mock-session-id", "fixed-session")

	require.Error(t, cmdErr)
	assert.Empty(t, string(output))
	assert.Contains(t, cmdErr.Error(), "--mock-session-id requires --mock")
}

func TestTickMock_GlobalNoOutputStillPrintsSessionID(t *testing.T) {
	t.Chdir(t.TempDir())

	output, cmdErr := executeTickCmd(t, sessiontest.NewMemoryStore(), "", "--agent", "claude-code", "--mock", "--global")
	require.NoError(t, cmdErr)

	sessionID := extractMockSessionID(t, string(output))
	assert.NotEmpty(t, sessionID)
	assert.Equal(t, "Argus: Mock session: "+sessionID+"\n", string(output))
}

func writeTickCommandWorkflowFixture(t *testing.T, projectRoot string) {
	t.Helper()
	workflowsDir := filepath.Join(projectRoot, ".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, "release.yaml"), []byte(`version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`), 0o600))
}
