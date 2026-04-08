package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeInstallCmd runs the install command and captures stdout output.
// Tests using this helper must NOT call t.Parallel since os.Stdout is redirected.
func executeInstallCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := newInstallCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	cmdErr := cmd.Execute()

	require.NoError(t, w.Close())
	os.Stdout = old

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return out, cmdErr
}

// executeUninstallCmd runs the uninstall command and captures stdout output.
// Tests using this helper must NOT call t.Parallel since os.Stdout is redirected.
func executeUninstallCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := newUninstallCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	cmdErr := cmd.Execute()

	require.NoError(t, w.Close())
	os.Stdout = old

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return out, cmdErr
}

func initGitRepo(t *testing.T) {
	t.Helper()
	require.NoError(t, os.MkdirAll(".git", 0o755))
}

func TestInstallLifecycle(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	initGitRepo(t)

	output, cmdErr := executeInstallCmd(t, "--yes")
	require.NoError(t, cmdErr)

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	assert.Equal(t, "ok", data["status"])

	for _, dir := range []string{"workflows", "invariants", "rules", "pipelines", "logs", "data", "tmp"} {
		_, err := os.Stat(filepath.Join(".argus", dir))
		assert.NoError(t, err, ".argus/%s should exist", dir)
	}

	_, err := os.Stat(filepath.Join(".argus", "workflows", "argus-init.yaml"))
	assert.NoError(t, err, "argus-init.yaml should exist")

	skillEntries, err := os.ReadDir(filepath.Join(".agents", "skills"))
	require.NoError(t, err)
	assert.Len(t, skillEntries, 9, "should have 9 argus-* skill directories")

	_, err = os.Stat(filepath.Join(".agents", "skills", "argus-doctor", "SKILL.md"))
	assert.NoError(t, err, "argus-doctor/SKILL.md should exist")

	output, cmdErr = executeUninstallCmd(t, "--yes")
	require.NoError(t, cmdErr)

	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	assert.Equal(t, "ok", data["status"])

	_, err = os.Stat(".argus")
	assert.True(t, os.IsNotExist(err), ".argus/ should not exist after uninstall")

	_, err = os.Stat(filepath.Join(".agents", "skills", "argus-doctor"))
	assert.True(t, os.IsNotExist(err), "argus-doctor/ should not exist after uninstall")

	output, cmdErr = executeInstallCmd(t, "--yes")
	require.NoError(t, cmdErr)

	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	assert.Equal(t, "ok", data["status"])

	_, err = os.Stat(filepath.Join(".argus", "workflows"))
	assert.NoError(t, err, ".argus/workflows should exist after reinstall")
}

func TestInstallEdgeCases(t *testing.T) {
	t.Run("non-git directory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())

		output, cmdErr := executeInstallCmd(t, "--yes")
		assert.Error(t, cmdErr)

		var data map[string]any
		require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
		assert.Equal(t, "error", data["status"])

		msg, ok := data["message"].(string)
		require.True(t, ok, "message should be a string")
		assert.True(t, strings.Contains(strings.ToLower(msg), "git"),
			"error message should mention git, got: %s", msg)
	})

	t.Run("nested install prevention", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())
		initGitRepo(t)

		_, cmdErr := executeInstallCmd(t, "--yes")
		require.NoError(t, cmdErr)

		subdir := filepath.Join("sub", "dir")
		require.NoError(t, os.MkdirAll(subdir, 0o755))
		t.Chdir(subdir)

		output, cmdErr := executeInstallCmd(t, "--yes")
		assert.Error(t, cmdErr)

		var data map[string]any
		require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
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

		var data map[string]any
		require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
		assert.Equal(t, "ok", data["status"])

		settingsData, err := os.ReadFile(filepath.Join(".claude", "settings.json"))
		require.NoError(t, err)

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
		require.NoError(t, os.MkdirAll(customSkillDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(customSkillDir, "SKILL.md"),
			[]byte("# My Custom Skill\n"),
			0o644,
		))

		_, cmdErr = executeUninstallCmd(t, "--yes")
		require.NoError(t, cmdErr)

		_, err := os.Stat(filepath.Join(".agents", "skills", "my-custom", "SKILL.md"))
		assert.NoError(t, err, "non-argus skill should be preserved after uninstall")

		_, err = os.Stat(filepath.Join(".agents", "skills", "argus-doctor"))
		assert.True(t, os.IsNotExist(err), "argus-doctor/ should be removed after uninstall")
	})

	t.Run("hook preservation", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())
		initGitRepo(t)

		require.NoError(t, os.MkdirAll(".claude", 0o755))
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
			0o644,
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
