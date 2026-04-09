package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/install"
	workspacecfg "github.com/nextzhou/argus/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeWorkspaceInstallCmd(t *testing.T, workspacePath string) ([]byte, error) {
	return executeWorkspaceInstallCmdWithArgs(t, workspacePath, "--yes")
}

func executeWorkspaceInstallCmdWithArgs(t *testing.T, workspacePath string, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newInstallCmd(), append(args, "--workspace", workspacePath)...)
}

func executeWorkspaceUninstallCmd(t *testing.T, workspacePath string) ([]byte, error) {
	return executeWorkspaceUninstallCmdWithArgs(t, workspacePath, "--yes")
}

func executeWorkspaceUninstallCmdWithArgs(t *testing.T, workspacePath string, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newUninstallCmd(), append(args, "--workspace", workspacePath)...)
}

func executeGlobalTickCmd(t *testing.T, stdinJSON string) ([]byte, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := newTickCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--agent", "claude-code", "--global"})
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

func initGitRepoAt(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(path, ".git"), 0o755))
}

func parseWorkspaceLifecycleOutput(t *testing.T, output []byte) map[string]any {
	t.Helper()

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	return data
}

func readWorkspaceConfig(t *testing.T) *workspacecfg.Config {
	t.Helper()

	config, err := workspacecfg.LoadConfig(install.UserConfigPath())
	require.NoError(t, err)
	return config
}

func readFileString(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func readOptionalFileString(t *testing.T, path string) (string, bool) {
	t.Helper()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	require.NoError(t, err)
	return string(data), true
}

func assertGlobalSkillState(t *testing.T, wantPresent bool) {
	t.Helper()

	for _, skillPath := range install.GlobalSkillPaths() {
		for _, skillName := range install.GlobalSkillNames() {
			skillFile := filepath.Join(skillPath, skillName, "SKILL.md")
			_, err := os.Stat(skillFile)
			if wantPresent {
				assert.NoError(t, err, "%s should exist", skillFile)
				continue
			}
			assert.True(t, os.IsNotExist(err), "%s should not exist", skillFile)
		}
	}
}

func TestWorkspaceLifecycle_Complete(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	projectDir := filepath.Join(workspaceDir, "myproject")
	postUninstallProjectDir := filepath.Join(workspaceDir, "otherproject")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	require.NoError(t, os.MkdirAll(postUninstallProjectDir, 0o755))
	initGitRepoAt(t, projectDir)
	initGitRepoAt(t, postUninstallProjectDir)

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")

	output, cmdErr := executeWorkspaceInstallCmd(t, workspaceDir)
	require.NoError(t, cmdErr)
	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assert.Equal(t, "~/work", data["path"])
	assertLifecycleReportShape(t, data,
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
	output, cmdErr = executeGlobalTickCmd(t, `{"session_id":"test-session"}`)
	require.NoError(t, cmdErr)
	assertHookSafeTickText(t, string(output))
	assert.Contains(t, string(output), "argus install")
	assert.Contains(t, string(output), "argus-install")

	output, cmdErr = executeInstallCmd(t, "--yes")
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	_, err := os.Stat(filepath.Join(projectDir, ".argus"))
	assert.NoError(t, err, "%s should exist after project install", filepath.Join(projectDir, ".argus"))

	output, cmdErr = executeGlobalTickCmd(t, `{"session_id":"test-session"}`)
	require.NoError(t, cmdErr)
	assert.Empty(t, string(output))

	output, cmdErr = executeWorkspaceUninstallCmd(t, workspaceDir)
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data,
		"~/.config/argus/config.yaml",
		"~/.claude/settings.json",
		"~/.codex/hooks.json",
	)
	config = readWorkspaceConfig(t)
	assert.Empty(t, config.Workspaces)
	if settingsData, ok := readOptionalFileString(t, settingsPath); ok {
		assert.NotContains(t, settingsData, "argus tick")
		assert.NotContains(t, settingsData, "--global")
	}
	assertGlobalSkillState(t, false)

	t.Chdir(postUninstallProjectDir)
	output, cmdErr = executeGlobalTickCmd(t, `{"session_id":"test-session"}`)
	require.NoError(t, cmdErr)
	assert.Empty(t, string(output))
}

func TestWorkspaceLifecycle_MultiWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceAlpha := filepath.Join(homeDir, "ws-alpha")
	workspaceBeta := filepath.Join(homeDir, "ws-beta")
	require.NoError(t, os.MkdirAll(workspaceAlpha, 0o755))
	require.NoError(t, os.MkdirAll(workspaceBeta, 0o755))

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")

	output, cmdErr := executeWorkspaceInstallCmd(t, workspaceAlpha)
	require.NoError(t, cmdErr)
	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/config.yaml")

	output, cmdErr = executeWorkspaceInstallCmd(t, workspaceBeta)
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/config.yaml")
	config := readWorkspaceConfig(t)
	assert.Equal(t, []string{"~/ws-alpha", "~/ws-beta"}, config.Workspaces)

	output, cmdErr = executeWorkspaceUninstallCmd(t, workspaceAlpha)
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

	output, cmdErr = executeWorkspaceUninstallCmd(t, workspaceBeta)
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/config.yaml", "~/.claude/settings.json")
	config = readWorkspaceConfig(t)
	assert.Empty(t, config.Workspaces)
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
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))
	t.Chdir(baseDir)

	output, cmdErr := executeWorkspaceInstallCmd(t, "./myworkspace")
	require.NoError(t, cmdErr)
	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/config.yaml")
	config := readWorkspaceConfig(t)
	assert.Equal(t, []string{workspaceDir}, config.Workspaces)

	output, cmdErr = executeWorkspaceUninstallCmd(t, workspaceDir)
	require.NoError(t, cmdErr)
	data = parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data, "~/.config/argus/config.yaml")
	config = readWorkspaceConfig(t)
	assert.Empty(t, config.Workspaces)
}

func TestWorkspaceInstallDuplicateRegistrationIsNoOp(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	_, cmdErr := executeWorkspaceInstallCmd(t, workspaceDir)
	require.NoError(t, cmdErr)

	output, cmdErr := executeWorkspaceInstallCmdWithArgs(t, workspaceDir)
	require.NoError(t, cmdErr)
	data := parseWorkspaceLifecycleOutput(t, output)
	assert.Equal(t, "workspace already registered", data["message"])
	assertEmptyLifecycleChanges(t, data)
	assertLifecycleReportShape(t, data, "~/.config/argus/config.yaml")
}

func TestWorkspaceInstallNonInteractiveWithoutYes(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	withPipeStdin(t, "", func() {
		output, cmdErr := executeWorkspaceInstallCmdWithArgs(t, workspaceDir)
		require.Error(t, cmdErr)
		assert.Contains(t, cmdErr.Error(), "--yes")

		data := parseWorkspaceLifecycleOutput(t, output)
		assert.Equal(t, "error", data["status"])
		assert.Contains(t, data["message"], "use --yes")
	})
}

func TestWorkspaceUninstallNonInteractiveWithoutYes(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))
	_, cmdErr := executeWorkspaceInstallCmd(t, workspaceDir)
	require.NoError(t, cmdErr)

	withPipeStdin(t, "", func() {
		output, cmdErr := executeWorkspaceUninstallCmdWithArgs(t, workspaceDir)
		require.Error(t, cmdErr)
		assert.Contains(t, cmdErr.Error(), "--yes")

		data := parseWorkspaceLifecycleOutput(t, output)
		assert.Equal(t, "error", data["status"])
		assert.Contains(t, data["message"], "use --yes")
	})
}
