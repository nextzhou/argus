package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/install"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeInstallCmd runs the install command and captures stdout output.
func executeInstallCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newInstallCmd(), args...)
}

// executeUninstallCmd runs the uninstall command and captures stdout output.
func executeUninstallCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newUninstallCmd(), args...)
}

func executeUninstallCmdWithInput(t *testing.T, input *bytes.Buffer, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommandWithInput(t, newUninstallCmd(), input, args...)
}

func initGitRepo(t *testing.T) {
	t.Helper()
	require.NoError(t, os.MkdirAll(".git", 0o700))
}

func parseLifecycleOutput(t *testing.T, output []byte) map[string]any {
	t.Helper()

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	return data
}

func assertLifecycleReportShape(t *testing.T, data map[string]any, expectedAffectedPaths ...string) {
	t.Helper()

	changes, ok := data["changes"].(map[string]any)
	require.True(t, ok, "changes should be an object")
	for _, key := range []string{"created", "updated", "removed"} {
		_, ok := changes[key].([]any)
		require.True(t, ok, "%s should be an array", key)
	}

	affected, ok := data["affected_paths"].([]any)
	require.True(t, ok, "affected_paths should be an array")
	gotAffected := make([]string, 0, len(affected))
	for _, value := range affected {
		asString, ok := value.(string)
		require.True(t, ok, "affected_paths values should be strings")
		gotAffected = append(gotAffected, asString)
	}

	for _, expected := range expectedAffectedPaths {
		assert.Contains(t, gotAffected, expected)
	}
}

func assertEmptyLifecycleChanges(t *testing.T, data map[string]any) {
	t.Helper()

	changes, ok := data["changes"].(map[string]any)
	require.True(t, ok, "changes should be an object")
	for _, key := range []string{"created", "updated", "removed"} {
		entries, ok := changes[key].([]any)
		require.True(t, ok, "%s should be an array", key)
		assert.Empty(t, entries, "%s should be empty", key)
	}
}

func TestInstallLifecycle(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	initGitRepo(t)

	output, cmdErr := executeInstallCmd(t, "--yes")
	require.NoError(t, cmdErr)

	data := parseLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data,
		".argus/{workflows,invariants,rules,pipelines,logs,data,tmp}/",
		".agents/skills/argus-*/SKILL.md",
		".claude/settings.json",
	)

	for _, dir := range []string{"workflows", "invariants", "rules", "pipelines", "logs", "data", "tmp"} {
		_, err := os.Stat(filepath.Join(".argus", dir))
		require.NoError(t, err, ".argus/%s should exist", dir)
	}

	_, err := os.Stat(filepath.Join(".argus", "workflows", "argus-init.yaml"))
	require.NoError(t, err, "argus-init.yaml should exist")

	skillEntries, err := os.ReadDir(filepath.Join(".agents", "skills"))
	require.NoError(t, err)
	assert.Len(t, skillEntries, 10, "should have 10 argus-* skill directories")

	for _, skillPath := range install.SkillPaths() {
		_, err = os.Stat(filepath.Join(skillPath, "argus-doctor", "SKILL.md"))
		require.NoError(t, err, "%s/argus-doctor/SKILL.md should exist", skillPath)
	}

	output, cmdErr = executeUninstallCmd(t, "--yes")
	require.NoError(t, cmdErr)

	data = parseLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data,
		".argus/",
		".agents/skills/argus-*",
		".claude/settings.json",
	)

	_, err = os.Stat(".argus")
	assert.True(t, os.IsNotExist(err), ".argus/ should not exist after uninstall")

	for _, skillPath := range install.SkillPaths() {
		_, err = os.Stat(filepath.Join(skillPath, "argus-doctor"))
		assert.True(t, os.IsNotExist(err), "%s/argus-doctor should not exist after uninstall", skillPath)
	}

	output, cmdErr = executeInstallCmd(t, "--yes")
	require.NoError(t, cmdErr)

	data = parseLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])

	_, err = os.Stat(filepath.Join(".argus", "workflows"))
	assert.NoError(t, err, ".argus/workflows should exist after reinstall")
}

func TestInstallEdgeCases(t *testing.T) {
	t.Run("non-git directory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())

		output, cmdErr := executeInstallCmd(t, "--yes")
		require.Error(t, cmdErr)

		data := parseLifecycleOutput(t, output)
		assert.Equal(t, "error", data["status"])

		msg, ok := data["message"].(string)
		require.True(t, ok, "message should be a string")
		assert.Contains(t, strings.ToLower(msg), "git",
			"error message should mention git, got: %s", msg)
	})

	t.Run("nested install prevention", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())
		initGitRepo(t)

		_, cmdErr := executeInstallCmd(t, "--yes")
		require.NoError(t, cmdErr)

		subdir := filepath.Join("sub", "dir")
		require.NoError(t, os.MkdirAll(subdir, 0o700))
		t.Chdir(subdir)

		output, cmdErr := executeInstallCmd(t, "--yes")
		require.Error(t, cmdErr)

		data := parseLifecycleOutput(t, output)
		assert.Equal(t, "error", data["status"])

		msg, ok := data["message"].(string)
		require.True(t, ok, "message should be a string")
		assert.Contains(t, msg, ".argus", "error should mention ancestor .argus/")
	})

	t.Run("idempotent install", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())
		initGitRepo(t)

		_, cmdErr := executeInstallCmd(t, "--yes")
		require.NoError(t, cmdErr)

		output, cmdErr := executeInstallCmd(t, "--yes")
		require.NoError(t, cmdErr)

		data := parseLifecycleOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assertEmptyLifecycleChanges(t, data)
		assertLifecycleReportShape(t, data, ".argus/{workflows,invariants,rules,pipelines,logs,data,tmp}/")

		settingsData, err := os.ReadFile(filepath.Join(".claude", "settings.json"))
		require.NoError(t, err)
		assert.NotContains(t, string(settingsData), "argus trap", "project install should not configure argus trap")

		var settings map[string]any
		require.NoError(t, json.Unmarshal(settingsData, &settings))

		hooks, ok := settings["hooks"].(map[string]any)
		require.True(t, ok, "hooks should be an object")

		userPromptEntries, ok := hooks["UserPromptSubmit"].([]any)
		require.True(t, ok, "UserPromptSubmit should be an array")

		argusCount := 0
		for _, entry := range userPromptEntries {
			entryJSON, err := json.Marshal(entry)
			require.NoError(t, err)
			if strings.Contains(string(entryJSON), "argus tick") {
				argusCount++
			}
		}
		assert.Equal(t, 1, argusCount, "should have exactly one argus tick entry after double install")
	})

	t.Run("selective skill cleanup", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())
		initGitRepo(t)

		_, cmdErr := executeInstallCmd(t, "--yes")
		require.NoError(t, cmdErr)

		customSkillDir := filepath.Join(".agents", "skills", "my-custom")
		require.NoError(t, os.MkdirAll(customSkillDir, 0o700))
		require.NoError(t, os.WriteFile(
			filepath.Join(customSkillDir, "SKILL.md"),
			[]byte("# My Custom Skill\n"),
			0o600,
		))

		claudeCustomSkillDir := filepath.Join(".claude", "skills", "my-custom")
		require.NoError(t, os.MkdirAll(claudeCustomSkillDir, 0o700))
		require.NoError(t, os.WriteFile(
			filepath.Join(claudeCustomSkillDir, "SKILL.md"),
			[]byte("# My Claude Custom Skill\n"),
			0o600,
		))

		_, cmdErr = executeUninstallCmd(t, "--yes")
		require.NoError(t, cmdErr)

		_, err := os.Stat(filepath.Join(".agents", "skills", "my-custom", "SKILL.md"))
		require.NoError(t, err, "non-argus skill should be preserved after uninstall")

		_, err = os.Stat(filepath.Join(".claude", "skills", "my-custom", "SKILL.md"))
		require.NoError(t, err, "non-argus Claude skill should be preserved after uninstall")

		for _, skillPath := range install.SkillPaths() {
			_, err = os.Stat(filepath.Join(skillPath, "argus-doctor"))
			assert.True(t, os.IsNotExist(err), "%s/argus-doctor should be removed after uninstall", skillPath)
		}
	})

	t.Run("hook preservation", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())
		initGitRepo(t)

		require.NoError(t, os.MkdirAll(".claude", 0o700))
		preExisting := map[string]any{
			"hooks": map[string]any{
				"UserPromptSubmit": []any{
					map[string]any{
						"hooks": []any{
							map[string]any{
								"type":    "command",
								"command": "my-other-tool",
								"timeout": float64(5),
							},
						},
					},
				},
			},
		}
		preExistingJSON, err := json.MarshalIndent(preExisting, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(
			filepath.Join(".claude", "settings.json"),
			preExistingJSON,
			0o600,
		))

		_, cmdErr := executeInstallCmd(t, "--yes")
		require.NoError(t, cmdErr)

		settingsData, err := os.ReadFile(filepath.Join(".claude", "settings.json"))
		require.NoError(t, err)

		var settings map[string]any
		require.NoError(t, json.Unmarshal(settingsData, &settings))

		hooks, ok := settings["hooks"].(map[string]any)
		require.True(t, ok, "hooks should be an object")

		entries, ok := hooks["UserPromptSubmit"].([]any)
		require.True(t, ok, "UserPromptSubmit should be an array")

		settingsStr := string(settingsData)
		assert.Contains(t, settingsStr, "my-other-tool", "non-argus hook should be present after install")
		assert.Contains(t, settingsStr, "argus tick", "argus hook should be present after install")
		assert.GreaterOrEqual(t, len(entries), 2, "should have at least 2 hook entries (non-argus + argus)")

		_, cmdErr = executeUninstallCmd(t, "--yes")
		require.NoError(t, cmdErr)

		settingsData, err = os.ReadFile(filepath.Join(".claude", "settings.json"))
		require.NoError(t, err)

		require.NoError(t, json.Unmarshal(settingsData, &settings))

		_, ok = settings["hooks"].(map[string]any)
		require.True(t, ok, "hooks should still be an object")

		settingsStr = string(settingsData)
		assert.Contains(t, settingsStr, "my-other-tool", "non-argus hook should be preserved after uninstall")
		assert.NotContains(t, settingsStr, "argus tick", "argus tick hook should be removed after uninstall")
		assert.NotContains(t, settingsStr, "argus trap", "argus trap hook should be removed after uninstall")
	})
}
