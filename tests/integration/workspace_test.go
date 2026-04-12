package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspace_CompleteLifecycle(t *testing.T) {
	sessionID := newDefaultSessionID(t, "ws-lifecycle")
	postTeardownSessionID := newDefaultSessionID(t, "post-teardown")
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	projectDir := filepath.Join(workspaceDir, "myproject")
	require.NoError(t, os.MkdirAll(projectDir, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".git"), 0o700))

	result := runArgusJSON(t, projectDir, "setup", "--workspace", workspaceDir, "--yes")
	data := requireOK(t, result)
	assert.Equal(t, "~/work", data["path"])

	configPath := filepath.Join(homeDir, ".config", "argus", "config.yaml")
	require.True(t, fileExists(t, configPath), "global config should exist")

	globalSettingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	if fileExists(t, globalSettingsPath) {
		//nolint:gosec // The test reads a settings file created under its temp HOME directory.
		content, err := os.ReadFile(globalSettingsPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "argus tick")
		assert.Contains(t, string(content), "--global")
	}

	for _, skillName := range []string{"argus-intro", "argus-setup", "argus-teardown", "argus-doctor"} {
		skillPath := filepath.Join(homeDir, ".claude", "skills", skillName, "SKILL.md")
		assert.True(t, fileExists(t, skillPath), "%s should exist", skillPath)
	}

	stdinJSON := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "argus setup")

	result = runArgusJSON(t, projectDir, "setup", "--yes")
	requireOK(t, result)
	require.True(t, fileExists(t, filepath.Join(projectDir, ".argus")))

	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "argus-project-init")
	assert.NotContains(t, result.Stdout, "argus-project-setup")

	writeFile(t, projectDir, ".argus/workflows/ws-test.yaml", `version: v0.1.0
id: ws-test
jobs:
  - id: step_one
    prompt: "Do step one"
`)
	result = runArgusJSON(t, projectDir, "workflow", "start", "ws-test")
	requireOK(t, result)

	stdinJSON = fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "step_one")

	result = runArgusJSON(t, projectDir, "teardown", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	uninitProjectDir := filepath.Join(workspaceDir, "otherproject")
	require.NoError(t, os.MkdirAll(filepath.Join(uninitProjectDir, ".git"), 0o700))
	stdinJSON = fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, postTeardownSessionID, uninitProjectDir)
	result = runArgusWithStdin(t, uninitProjectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Stdout, "global tick should produce nothing after workspace teardown")
}

func TestWorkspace_NonGitDirSkip(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	nonGitDir := filepath.Join(workspaceDir, "not-a-repo")
	require.NoError(t, os.MkdirAll(nonGitDir, 0o700))

	result := runArgusJSON(t, nonGitDir, "setup", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	sessionID := newDefaultSessionID(t, "non-git")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, nonGitDir)
	result = runArgusWithStdin(t, nonGitDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Stdout, "global tick should skip non-git directory")
}

func TestWorkspace_GitFileMarkerUsesWorkspaceAndProjectScopes(t *testing.T) {
	sessionID := newDefaultSessionID(t, "git-file-marker")
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	projectDir := filepath.Join(workspaceDir, "worktree-project")
	require.NoError(t, os.MkdirAll(projectDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".git"), []byte("gitdir: /tmp/worktrees/example\n"), 0o600))

	result := runArgusJSON(t, projectDir, "setup", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	stdinJSON := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "argus setup")
	assert.Contains(t, result.Stdout, "argus-project-setup")

	result = runArgusJSON(t, projectDir, "setup", "--yes")
	requireOK(t, result)

	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "argus-project-init")
	assert.NotContains(t, result.Stdout, "argus-project-setup")
}

func TestWorkspace_OutsideWorkspace(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	outsideDir := filepath.Join(homeDir, "outside")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(outsideDir, ".git"), 0o700))

	result := runArgusJSON(t, outsideDir, "setup", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	sessionID := newDefaultSessionID(t, "outside-workspace")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, outsideDir)
	result = runArgusWithStdin(t, outsideDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Stdout, "global tick should skip project outside workspace")
}

func TestWorkspace_MultiWorkspace(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	wsAlpha := filepath.Join(homeDir, "ws-alpha")
	wsBeta := filepath.Join(homeDir, "ws-beta")
	require.NoError(t, os.MkdirAll(wsAlpha, 0o700))
	require.NoError(t, os.MkdirAll(wsBeta, 0o700))

	result := runArgusJSON(t, homeDir, "setup", "--workspace", wsAlpha, "--yes")
	requireOK(t, result)

	result = runArgusJSON(t, homeDir, "setup", "--workspace", wsBeta, "--yes")
	requireOK(t, result)

	globalSettingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	for _, skillName := range []string{"argus-intro", "argus-setup", "argus-teardown", "argus-doctor"} {
		skillPath := filepath.Join(homeDir, ".claude", "skills", skillName, "SKILL.md")
		assert.True(t, fileExists(t, skillPath), "%s should exist after two workspace registrations", skillPath)
	}

	result = runArgusJSON(t, homeDir, "teardown", "--workspace", wsAlpha, "--yes")
	requireOK(t, result)

	if fileExists(t, globalSettingsPath) {
		//nolint:gosec // The test reads a settings file created under its temp HOME directory.
		content, err := os.ReadFile(globalSettingsPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "argus tick",
			"global hooks should remain while one workspace is registered")
	}
	for _, skillName := range []string{"argus-intro", "argus-setup", "argus-teardown", "argus-doctor"} {
		skillPath := filepath.Join(homeDir, ".claude", "skills", skillName, "SKILL.md")
		assert.True(t, fileExists(t, skillPath),
			"%s should still exist with remaining workspace", skillPath)
	}

	result = runArgusJSON(t, homeDir, "teardown", "--workspace", wsBeta, "--yes")
	requireOK(t, result)

	for _, skillName := range []string{"argus-intro", "argus-setup", "argus-teardown", "argus-doctor"} {
		skillPath := filepath.Join(homeDir, ".claude", "skills", skillName, "SKILL.md")
		assert.False(t, fileExists(t, skillPath),
			"%s should be removed after last workspace teardown", skillPath)
	}
}

func TestWorkspace_TeardownNotRegistered(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	result := runArgusJSON(t, homeDir, "teardown", "--workspace", "/nonexistent/path")
	data := requireError(t, result)
	assert.Contains(t, mustJSONString(t, data["message"]), "not registered")
}

func TestWorkspace_DuplicateRegistration(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))

	result := runArgusJSON(t, homeDir, "setup", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	result = runArgusJSON(t, homeDir, "setup", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	configPath := filepath.Join(homeDir, ".config", "argus", "config.yaml")
	//nolint:gosec // The test reads a config file created under its temp HOME directory.
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "~/work")
}

func TestWorkspace_PathNormalization(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))

	result := runArgusJSON(t, homeDir, "setup", "--workspace", workspaceDir, "--yes")
	data := requireOK(t, result)
	assert.Equal(t, "~/work", data["path"])

	result = runArgusJSON(t, homeDir, "teardown", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)
}

func TestWorkspace_GlobalTickGuidesMention(t *testing.T) {
	sessionID := newDefaultSessionID(t, "guide")
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	projectDir := filepath.Join(workspaceDir, "guide-project")
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".git"), 0o700))

	result := runArgusJSON(t, homeDir, "setup", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	stdinJSON := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "argus setup",
		"guidance should mention argus setup command")
	assert.Contains(t, result.Stdout, "argus-setup",
		"guidance should mention argus-setup skill")
	assert.Contains(t, result.Stdout, "argus-intro",
		"guidance should mention argus-intro skill")
}

func TestWorkspace_SubAgentSkipGlobalTick(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	projectDir := filepath.Join(workspaceDir, "sub-agent-project")
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".git"), 0o700))

	result := runArgusJSON(t, homeDir, "setup", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	sessionID := newDefaultSessionID(t, "sub-agent-global")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s","agent_id":"worker-1","cwd":"%s"}`, sessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Stdout, "sub-agent should produce no output even in global tick")
}

func TestWorkspace_SetupBadPath(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	result := runArgusJSON(t, homeDir, "setup", "--workspace", "/nonexistent/workspace/path")
	data := requireError(t, result)
	assert.Contains(t, mustJSONString(t, data["message"]), "does not exist")
}
