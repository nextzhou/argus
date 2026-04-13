package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/lifecycle"
	"github.com/nextzhou/argus/internal/session"
	"github.com/nextzhou/argus/internal/sessiontest"
	workspacecfg "github.com/nextzhou/argus/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeWorkspaceSetupCmd(t *testing.T, workspacePath string) ([]byte, error) {
	t.Helper()
	return executeWorkspaceSetupCmdWithArgs(t, workspacePath, "--yes")
}

func executeWorkspaceSetupCmdWithArgs(t *testing.T, workspacePath string, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newSetupCmd(), append(args, "--workspace", workspacePath)...)
}

func executeWorkspaceTeardownCmd(t *testing.T, workspacePath string) ([]byte, error) {
	t.Helper()
	return executeWorkspaceTeardownCmdWithArgs(t, workspacePath, "--yes")
}

func executeWorkspaceTeardownCmdWithArgs(t *testing.T, workspacePath string, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newTeardownCmd(), append(args, "--workspace", workspacePath)...)
}

func executeGlobalTickCmd(t *testing.T, store session.Store, stdinJSON string) ([]byte, error) {
	t.Helper()

	var out bytes.Buffer

	cmd := newTickCmdWithSessionStore(store)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--agent", "claude-code", "--global"})
	if stdinJSON != "" {
		cmd.SetIn(strings.NewReader(stdinJSON))
	}
	cmdErr := cmd.Execute()

	return out.Bytes(), cmdErr
}

func initGitRepoAt(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(path, ".git"), 0o700))
}

func parseWorkspaceLifecycleOutput(t *testing.T, output []byte) map[string]any {
	t.Helper()

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	return data
}

func readWorkspaceConfig(t *testing.T) *workspacecfg.Config {
	t.Helper()

	config, err := workspacecfg.LoadConfig(lifecycle.UserConfigPath())
	require.NoError(t, err)
	return config
}

func assertWorkspaceConfigMissing(t *testing.T) {
	t.Helper()

	_, err := os.Stat(lifecycle.UserConfigPath())
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Dir(lifecycle.UserConfigPath()))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func readFileString(t *testing.T, path string) string {
	t.Helper()

	//nolint:gosec // Test reads a file from its controlled temp workspace.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func readOptionalFileString(t *testing.T, path string) (string, bool) {
	t.Helper()

	//nolint:gosec // Test reads a file from its controlled temp workspace.
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	require.NoError(t, err)
	return string(data), true
}

func assertGlobalSkillState(t *testing.T, wantPresent bool) {
	t.Helper()

	for _, skillPath := range lifecycle.GlobalSkillPaths() {
		for _, skillName := range lifecycle.GlobalSkillNames() {
			skillFile := filepath.Join(skillPath, skillName, "SKILL.md")
			_, err := os.Stat(skillFile)
			if wantPresent {
				require.NoError(t, err, "%s should exist", skillFile)
				continue
			}
			assert.True(t, os.IsNotExist(err), "%s should not exist", skillFile)
		}
	}
}

func TestWorkspaceLifecycle_Complete(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	store := sessiontest.NewMemoryStore()
	sessionID := sessiontest.NewSessionID(t, "workspace-global")

	workspaceDir := filepath.Join(homeDir, "work")
	projectDir := filepath.Join(workspaceDir, "myproject")
	postTeardownProjectDir := filepath.Join(workspaceDir, "otherproject")
	require.NoError(t, os.MkdirAll(projectDir, 0o700))
	require.NoError(t, os.MkdirAll(postTeardownProjectDir, 0o700))
	initGitRepoAt(t, projectDir)
	initGitRepoAt(t, postTeardownProjectDir)

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")

	output, cmdErr := executeWorkspaceSetupCmd(t, workspaceDir)
	require.NoError(t, cmdErr)
	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assert.Equal(t, "~/work", data["path"])
	assertLifecycleReportShape(t, data,
		"~/.config/argus/{invariants,workflows,pipelines,logs}/",
		"~/.config/argus/config.yaml",
		"~/.claude/settings.json",
		"~/.codex/{hooks.json,config.toml}",
	)
	config := readWorkspaceConfig(t)
	assert.Equal(t, []string{"~/work"}, config.Workspaces)
	settingsData := readFileString(t, settingsPath)
	assert.Contains(t, settingsData, "argus tick")
	assert.Contains(t, settingsData, "--global")
	assertGlobalSkillState(t, true)

	t.Chdir(projectDir)
	output, cmdErr = executeGlobalTickCmd(t, store, mustJSONInput(t, map[string]string{
		"session_id": sessionID,
	}))
	require.NoError(t, cmdErr)
	assertHookSafeTickText(t, string(output))
	assert.Contains(t, string(output), "argus setup")
	assert.Contains(t, string(output), "argus-setup")

	output, cmdErr = executeSetupCmd(t, "--yes")
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	_, err := os.Stat(filepath.Join(projectDir, ".argus"))
	require.NoError(t, err, "%s should exist after project setup", filepath.Join(projectDir, ".argus"))

	output, cmdErr = executeGlobalTickCmd(t, store, mustJSONInput(t, map[string]string{
		"session_id": sessionID,
	}))
	require.NoError(t, cmdErr)
	assertHookSafeTickText(t, string(output))
	assert.Contains(t, string(output), "argus-project-init")
	assert.NotContains(t, string(output), "argus-project-setup")

	output, cmdErr = executeWorkspaceTeardownCmd(t, workspaceDir)
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data,
		"~/.config/argus/",
		"~/.config/argus/config.yaml",
		"~/.claude/settings.json",
		"~/.codex/hooks.json",
	)
	assertWorkspaceConfigMissing(t)
	if settingsData, ok := readOptionalFileString(t, settingsPath); ok {
		assert.NotContains(t, settingsData, "argus tick")
		assert.NotContains(t, settingsData, "--global")
	}
	assertGlobalSkillState(t, false)

	t.Chdir(postTeardownProjectDir)
	output, cmdErr = executeGlobalTickCmd(t, store, mustJSONInput(t, map[string]string{
		"session_id": sessionID,
	}))
	require.NoError(t, cmdErr)
	assert.Empty(t, string(output))
}

func TestWorkspaceLifecycle_MultiWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceAlpha := filepath.Join(homeDir, "ws-alpha")
	workspaceBeta := filepath.Join(homeDir, "ws-beta")
	require.NoError(t, os.MkdirAll(workspaceAlpha, 0o700))
	require.NoError(t, os.MkdirAll(workspaceBeta, 0o700))

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")

	output, cmdErr := executeWorkspaceSetupCmd(t, workspaceAlpha)
	require.NoError(t, cmdErr)
	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/{invariants,workflows,pipelines,logs}/", "~/.config/argus/config.yaml")

	output, cmdErr = executeWorkspaceSetupCmd(t, workspaceBeta)
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/{invariants,workflows,pipelines,logs}/", "~/.config/argus/config.yaml")
	config := readWorkspaceConfig(t)
	assert.Equal(t, []string{"~/ws-alpha", "~/ws-beta"}, config.Workspaces)

	output, cmdErr = executeWorkspaceTeardownCmd(t, workspaceAlpha)
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/config.yaml")
	config = readWorkspaceConfig(t)
	assert.Equal(t, []string{"~/ws-beta"}, config.Workspaces)
	settingsData := readFileString(t, settingsPath)
	assert.Contains(t, settingsData, "argus tick")
	assert.Contains(t, settingsData, "--global")
	assertGlobalSkillState(t, true)

	output, cmdErr = executeWorkspaceTeardownCmd(t, workspaceBeta)
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/", "~/.config/argus/config.yaml", "~/.claude/settings.json")
	assertWorkspaceConfigMissing(t)
	if settingsData, ok := readOptionalFileString(t, settingsPath); ok {
		assert.NotContains(t, settingsData, "argus tick")
		assert.NotContains(t, settingsData, "--global")
	}
	assertGlobalSkillState(t, false)
}

func TestWorkspaceLifecycle_PathNormalization(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	baseDir := filepath.Join(t.TempDir(), "test-normalization")
	workspaceDir := filepath.Join(baseDir, "myworkspace")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	t.Chdir(baseDir)

	output, cmdErr := executeWorkspaceSetupCmd(t, "./myworkspace")
	require.NoError(t, cmdErr)
	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/{invariants,workflows,pipelines,logs}/", "~/.config/argus/config.yaml")
	config := readWorkspaceConfig(t)
	assert.Equal(t, []string{workspaceDir}, config.Workspaces)

	output, cmdErr = executeWorkspaceTeardownCmd(t, workspaceDir)
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/", "~/.config/argus/config.yaml")
	assertWorkspaceConfigMissing(t)
}

func TestWorkspaceSetupDuplicateRegistrationIsNoOp(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))

	_, cmdErr := executeWorkspaceSetupCmd(t, workspaceDir)
	require.NoError(t, cmdErr)

	output, cmdErr := executeWorkspaceSetupCmdWithArgs(t, workspaceDir, "--yes")
	require.NoError(t, cmdErr)
	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "workspace already registered; global resources already up to date", data["message"])
	assertEmptyLifecycleChanges(t, data)
	assertLifecycleReportShape(t, data, "~/.config/argus/{invariants,workflows,pipelines,logs}/", "~/.config/argus/config.yaml")
}

func TestWorkspaceSetupDuplicateRegistrationRefreshesResources(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))

	_, cmdErr := executeWorkspaceSetupCmd(t, workspaceDir)
	require.NoError(t, cmdErr)

	for _, skillPath := range lifecycle.GlobalSkillPaths() {
		require.NoError(t, os.MkdirAll(filepath.Join(skillPath, "argus-concepts"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(skillPath, "argus-concepts", "SKILL.md"), []byte("# legacy\n"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".agents", "skills", "argus-intro", "SKILL.md"), []byte("# stale intro\n"), 0o600))

	output, cmdErr := executeWorkspaceSetupCmdWithArgs(t, workspaceDir, "--yes")
	require.NoError(t, cmdErr)
	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "workspace already registered; global resources refreshed", data["message"])
	assertLifecycleReportShape(t, data, "~/.config/argus/{invariants,workflows,pipelines,logs}/", "~/.config/argus/config.yaml")

	for _, skillPath := range lifecycle.GlobalSkillPaths() {
		_, err := os.Stat(filepath.Join(skillPath, "argus-concepts"))
		assert.True(t, os.IsNotExist(err), "%s/argus-concepts should be pruned", skillPath)
		_, err = os.Stat(filepath.Join(skillPath, "argus-intro", "SKILL.md"))
		assert.NoError(t, err, "%s/argus-intro/SKILL.md should exist", skillPath)
	}
}

func TestWorkspaceSetupJSONWithoutYes(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))

	output, cmdErr := executeJSONCommandWithInput(t, newSetupCmd(), bytes.NewBuffer(nil), "--workspace", workspaceDir)
	require.Error(t, cmdErr)
	assert.Equal(t, "workspace setup requires --yes when --json is used; --json is non-interactive", cmdErr.Error())

	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "error", data["status"])
	assert.Equal(t, "workspace setup requires --yes when --json is used; --json is non-interactive", data["message"])
}

func TestWorkspaceSetupDuplicateJSONWithoutYes(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	_, cmdErr := executeWorkspaceSetupCmd(t, workspaceDir)
	require.NoError(t, cmdErr)

	output, cmdErr := executeJSONCommandWithInput(t, newSetupCmd(), bytes.NewBuffer(nil), "--workspace", workspaceDir)
	require.Error(t, cmdErr)
	assert.Equal(t, "workspace setup requires --yes when --json is used; --json is non-interactive", cmdErr.Error())

	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "error", data["status"])
	assert.Equal(t, "workspace setup requires --yes when --json is used; --json is non-interactive", data["message"])
}

func TestWorkspaceTeardownJSONWithoutYes(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	_, cmdErr := executeWorkspaceSetupCmd(t, workspaceDir)
	require.NoError(t, cmdErr)

	output, cmdErr := executeJSONCommandWithInput(t, newTeardownCmd(), bytes.NewBuffer(nil), "--workspace", workspaceDir)
	require.Error(t, cmdErr)
	assert.Equal(t, "workspace teardown requires --yes when --json is used; --json is non-interactive", cmdErr.Error())

	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "error", data["status"])
	assert.Equal(t, "workspace teardown requires --yes when --json is used; --json is non-interactive", data["message"])
}
