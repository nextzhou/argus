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
	postUninstallSessionID := newDefaultSessionID(t, "post-uninstall")
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	projectDir := filepath.Join(workspaceDir, "myproject")
	require.NoError(t, os.MkdirAll(projectDir, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".git"), 0o700))

	result := runArgusJSON(t, projectDir, "install", "--workspace", workspaceDir, "--yes")
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

	for _, skillName := range []string{"argus-intro", "argus-install", "argus-uninstall", "argus-doctor"} {
		skillPath := filepath.Join(homeDir, ".claude", "skills", skillName, "SKILL.md")
		assert.True(t, fileExists(t, skillPath), "%s should exist", skillPath)
	}

	stdinJSON := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "argus install")

	result = runArgusJSON(t, projectDir, "install", "--yes")
	requireOK(t, result)
	require.True(t, fileExists(t, filepath.Join(projectDir, ".argus")))

	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Stdout)

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

	result = runArgusJSON(t, projectDir, "uninstall", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	uninitProjectDir := filepath.Join(workspaceDir, "otherproject")
	require.NoError(t, os.MkdirAll(filepath.Join(uninitProjectDir, ".git"), 0o700))
	stdinJSON = fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, postUninstallSessionID, uninitProjectDir)
	result = runArgusWithStdin(t, uninitProjectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Stdout, "global tick should produce nothing after workspace uninstall")
}

func TestWorkspace_NonGitDirSkip(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	nonGitDir := filepath.Join(workspaceDir, "not-a-repo")
	require.NoError(t, os.MkdirAll(nonGitDir, 0o700))

	result := runArgusJSON(t, nonGitDir, "install", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	sessionID := newDefaultSessionID(t, "non-git")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, nonGitDir)
	result = runArgusWithStdin(t, nonGitDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Stdout, "global tick should skip non-git directory")
}

func TestWorkspace_OutsideWorkspace(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	outsideDir := filepath.Join(homeDir, "outside")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(outsideDir, ".git"), 0o700))

	result := runArgusJSON(t, outsideDir, "install", "--workspace", workspaceDir, "--yes")
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

	result := runArgusJSON(t, homeDir, "install", "--workspace", wsAlpha, "--yes")
	requireOK(t, result)

	result = runArgusJSON(t, homeDir, "install", "--workspace", wsBeta, "--yes")
	requireOK(t, result)

	globalSettingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	for _, skillName := range []string{"argus-intro", "argus-install", "argus-uninstall", "argus-doctor"} {
		skillPath := filepath.Join(homeDir, ".claude", "skills", skillName, "SKILL.md")
		assert.True(t, fileExists(t, skillPath), "%s should exist after two workspace registrations", skillPath)
	}

	result = runArgusJSON(t, homeDir, "uninstall", "--workspace", wsAlpha, "--yes")
	requireOK(t, result)

	if fileExists(t, globalSettingsPath) {
		//nolint:gosec // The test reads a settings file created under its temp HOME directory.
		content, err := os.ReadFile(globalSettingsPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "argus tick",
			"global hooks should remain while one workspace is registered")
	}
	for _, skillName := range []string{"argus-intro", "argus-install", "argus-uninstall", "argus-doctor"} {
		skillPath := filepath.Join(homeDir, ".claude", "skills", skillName, "SKILL.md")
		assert.True(t, fileExists(t, skillPath),
			"%s should still exist with remaining workspace", skillPath)
	}

	result = runArgusJSON(t, homeDir, "uninstall", "--workspace", wsBeta, "--yes")
	requireOK(t, result)

	for _, skillName := range []string{"argus-intro", "argus-install", "argus-uninstall", "argus-doctor"} {
		skillPath := filepath.Join(homeDir, ".claude", "skills", skillName, "SKILL.md")
		assert.False(t, fileExists(t, skillPath),
			"%s should be removed after last workspace uninstall", skillPath)
	}
}

func TestWorkspace_UninstallNotRegistered(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	result := runArgusJSON(t, homeDir, "uninstall", "--workspace", "/nonexistent/path")
	data := requireError(t, result)
	assert.Contains(t, data["message"].(string), "not registered")
}

func TestWorkspace_DuplicateRegistration(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))

	result := runArgusJSON(t, homeDir, "install", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	result = runArgusJSON(t, homeDir, "install", "--workspace", workspaceDir, "--yes")
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

	result := runArgusJSON(t, homeDir, "install", "--workspace", workspaceDir, "--yes")
	data := requireOK(t, result)
	assert.Equal(t, "~/work", data["path"])

	result = runArgusJSON(t, homeDir, "uninstall", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)
}

func TestWorkspace_GlobalTickGuidesMention(t *testing.T) {
	sessionID := newDefaultSessionID(t, "guide")
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	projectDir := filepath.Join(workspaceDir, "guide-project")
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".git"), 0o700))

	result := runArgusJSON(t, homeDir, "install", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	stdinJSON := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "argus install",
		"guidance should mention argus install command")
	assert.Contains(t, result.Stdout, "argus-install",
		"guidance should mention argus-install skill")
	assert.Contains(t, result.Stdout, "argus-intro",
		"guidance should mention argus-intro skill")
}

func TestWorkspace_SubAgentSkipGlobalTick(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	projectDir := filepath.Join(workspaceDir, "sub-agent-project")
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".git"), 0o700))

	result := runArgusJSON(t, homeDir, "install", "--workspace", workspaceDir, "--yes")
	requireOK(t, result)

	sessionID := newDefaultSessionID(t, "sub-agent-global")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s","agent_id":"worker-1","cwd":"%s"}`, sessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code", "--global")
	require.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Stdout, "sub-agent should produce no output even in global tick")
}

func TestWorkspace_InstallBadPath(t *testing.T) {
	homeDir := resolveSymlinks(t, t.TempDir())
	t.Setenv("HOME", homeDir)

	result := runArgusJSON(t, homeDir, "install", "--workspace", "/nonexistent/workspace/path")
	data := requireError(t, result)
	assert.Contains(t, data["message"].(string), "does not exist")
}
