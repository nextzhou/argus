package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeTickCmd(t *testing.T, stdinJSON string, args ...string) ([]byte, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := newTickCmd()
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

func TestTickNoActivePipeline(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickCommandWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Chdir(oldCwd)) }()
	require.NoError(t, os.Chdir(projectRoot))

	output, cmdErr := executeTickCmd(t, `{"session_id":"tick-cli-no-pipeline","cwd":"`+projectRoot+`"}`, "--agent", "claude-code")
	require.NoError(t, cmdErr)
	assert.Contains(t, string(output), "[Argus]")
	assert.Contains(t, string(output), "argus workflow start")
}

func TestTickSubAgentSkip(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".argus", "workflows"), 0o755))

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Chdir(oldCwd)) }()
	require.NoError(t, os.Chdir(projectRoot))

	output, cmdErr := executeTickCmd(t, `{"session_id":"tick-cli-sub-agent","agent_id":"worker-1"}`, "--agent", "claude-code")
	require.NoError(t, cmdErr)
	assert.Empty(t, string(output))
}

func TestTickFailOpen(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".argus", "workflows"), 0o755))

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Chdir(oldCwd)) }()
	require.NoError(t, os.Chdir(projectRoot))

	output, cmdErr := executeTickCmd(t, `{invalid json}`, "--agent", "claude-code")
	require.NoError(t, cmdErr)
	assert.Contains(t, string(output), "[Argus] Warning")
	assert.Contains(t, string(output), "could not parse hook input")
}

func TestTickWithoutAgent(t *testing.T) {
	output, cmdErr := executeTickCmd(t, `{"session_id":"tick-cli-missing-agent"}`)
	require.Error(t, cmdErr)
	assert.Empty(t, string(output))
	assert.Contains(t, cmdErr.Error(), "required flag(s) \"agent\" not set")
}

func writeTickCommandWorkflowFixture(t *testing.T, projectRoot, workflowID, yamlContent string) {
	t.Helper()
	workflowsDir := filepath.Join(projectRoot, ".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, workflowID+".yaml"), []byte(yamlContent), 0o644))
}
