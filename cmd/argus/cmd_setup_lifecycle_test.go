package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/lifecycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeSetupCmd runs the setup command and captures stdout output.
func executeSetupCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newSetupCmd(), args...)
}

func executeSetupCmdWithInput(t *testing.T, input *bytes.Buffer, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommandWithInput(t, newSetupCmd(), input, args...)
}

// executeTeardownCmd runs the teardown command and captures stdout output.
func executeTeardownCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newTeardownCmd(), args...)
}

func executeTeardownCmdWithInput(t *testing.T, input *bytes.Buffer, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommandWithInput(t, newTeardownCmd(), input, args...)
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

	changes := mustJSONObject(t, data["changes"])
	for _, key := range []string{"created", "updated", "removed"} {
		mustJSONArray(t, changes[key])
	}

	affected := mustJSONArray(t, data["affected_paths"])
	gotAffected := make([]string, 0, len(affected))
	for _, value := range affected {
		asString := mustJSONString(t, value)
		gotAffected = append(gotAffected, asString)
	}

	for _, expected := range expectedAffectedPaths {
		assert.Contains(t, gotAffected, expected)
	}
}

func assertEmptyLifecycleChanges(t *testing.T, data map[string]any) {
	t.Helper()

	changes := mustJSONObject(t, data["changes"])
	for _, key := range []string{"created", "updated", "removed"} {
		entries := mustJSONArray(t, changes[key])
		assert.Empty(t, entries, "%s should be empty", key)
	}
}

func TestSetupLifecycle(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	initGitRepo(t)

	output, cmdErr := executeSetupCmd(t, "--yes")
	require.NoError(t, cmdErr)

	data := parseLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data,
		".argus/{workflows,invariants,rules,pipelines,logs,data,tmp}/",
		".agents/skills/argus-*/SKILL.md",
		"~/.agents/skills/argus-*/SKILL.md",
		".claude/settings.json",
	)

	for _, dir := range []string{"workflows", "invariants", "rules", "pipelines", "logs", "data", "tmp"} {
		_, err := os.Stat(filepath.Join(".argus", dir))
		require.NoError(t, err, ".argus/%s should exist", dir)
	}

	_, err := os.Stat(filepath.Join(".argus", "workflows", "argus-project-init.yaml"))
	require.NoError(t, err, "argus-project-init.yaml should exist")

	skillEntries, err := os.ReadDir(filepath.Join(".agents", "skills"))
	require.NoError(t, err)
	assert.Len(t, skillEntries, len(lifecycle.ProjectSkillNames()), "should have project-scope argus-* skill directories only")

	for _, skillPath := range lifecycle.SkillPaths() {
		for _, skillName := range lifecycle.ProjectSkillNames() {
			_, err = os.Stat(filepath.Join(skillPath, skillName, "SKILL.md"))
			require.NoError(t, err, "%s/%s/SKILL.md should exist", skillPath, skillName)
		}
		_, err = os.Stat(filepath.Join(skillPath, "argus-configure-workflow", "SKILL.md"))
		assert.True(t, os.IsNotExist(err), "%s/argus-configure-workflow/SKILL.md should not exist in project scope", skillPath)
	}
	for _, skillPath := range lifecycle.GlobalSkillPaths() {
		for _, skillName := range lifecycle.GlobalSkillNames() {
			_, err = os.Stat(filepath.Join(skillPath, skillName, "SKILL.md"))
			require.NoError(t, err, "%s/%s/SKILL.md should exist", skillPath, skillName)
		}
	}

	output, cmdErr = executeTeardownCmd(t, "--yes")
	require.NoError(t, cmdErr)

	data = parseLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])
	assertLifecycleReportShape(t, data,
		".argus/",
		".agents/skills/argus-*",
		".claude/settings.json",
	)

	_, err = os.Stat(".argus")
	assert.True(t, os.IsNotExist(err), ".argus/ should not exist after teardown")

	for _, skillPath := range lifecycle.SkillPaths() {
		_, err = os.Stat(filepath.Join(skillPath, "argus-doctor"))
		assert.True(t, os.IsNotExist(err), "%s/argus-doctor should not exist after teardown", skillPath)
	}
	for _, skillPath := range lifecycle.GlobalSkillPaths() {
		_, err = os.Stat(filepath.Join(skillPath, "argus-runtime", "SKILL.md"))
		require.NoError(t, err, "%s/argus-runtime/SKILL.md should survive project teardown", skillPath)
	}

	output, cmdErr = executeSetupCmd(t, "--yes")
	require.NoError(t, cmdErr)

	data = parseLifecycleOutput(t, output)
	assert.Equal(t, "ok", data["status"])

	_, err = os.Stat(filepath.Join(".argus", "workflows"))
	assert.NoError(t, err, ".argus/workflows should exist after repeated setup")
}

func TestSetupEdgeCases(t *testing.T) {
	t.Run("non-git directory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())

		output, cmdErr := executeSetupCmd(t, "--yes")
		require.Error(t, cmdErr)

		data := parseLifecycleOutput(t, output)
		assert.Equal(t, "error", data["status"])

		msg := mustJSONString(t, data["message"])
		assert.Contains(t, strings.ToLower(msg), "git",
			"error message should mention git, got: %s", msg)
	})

	t.Run("nested setup prevention", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())
		initGitRepo(t)

		_, cmdErr := executeSetupCmd(t, "--yes")
		require.NoError(t, cmdErr)

		subdir := filepath.Join("sub", "dir")
		require.NoError(t, os.MkdirAll(subdir, 0o700))
		t.Chdir(subdir)

		output, cmdErr := executeSetupCmd(t, "--yes")
		require.Error(t, cmdErr)

		data := parseLifecycleOutput(t, output)
		assert.Equal(t, "error", data["status"])

		msg := mustJSONString(t, data["message"])
		assert.Contains(t, msg, ".argus", "error should mention ancestor .argus/")
	})

	t.Run("idempotent setup", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())
		initGitRepo(t)

		_, cmdErr := executeSetupCmd(t, "--yes")
		require.NoError(t, cmdErr)

		output, cmdErr := executeSetupCmd(t, "--yes")
		require.NoError(t, cmdErr)

		data := parseLifecycleOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assertEmptyLifecycleChanges(t, data)
		assertLifecycleReportShape(t, data, ".argus/{workflows,invariants,rules,pipelines,logs,data,tmp}/", "~/.agents/skills/argus-*/SKILL.md")

		settingsData, err := os.ReadFile(filepath.Join(".claude", "settings.json"))
		require.NoError(t, err)
		assert.NotContains(t, string(settingsData), "argus trap", "project setup should not configure argus trap")

		var settings map[string]any
		require.NoError(t, json.Unmarshal(settingsData, &settings))

		hooks := mustJSONObject(t, settings["hooks"])

		userPromptEntries := mustJSONArray(t, hooks["UserPromptSubmit"])

		argusCount := 0
		for _, entry := range userPromptEntries {
			entryJSON, err := json.Marshal(entry)
			require.NoError(t, err)
			if strings.Contains(string(entryJSON), "argus tick") {
				argusCount++
			}
		}
		assert.Equal(t, 1, argusCount, "should have exactly one argus tick entry after double setup")
	})

	t.Run("selective skill cleanup", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("HOME", t.TempDir())
		initGitRepo(t)

		_, cmdErr := executeSetupCmd(t, "--yes")
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

		_, cmdErr = executeTeardownCmd(t, "--yes")
		require.NoError(t, cmdErr)

		_, err := os.Stat(filepath.Join(".agents", "skills", "my-custom", "SKILL.md"))
		require.NoError(t, err, "non-argus skill should be preserved after teardown")

		_, err = os.Stat(filepath.Join(".claude", "skills", "my-custom", "SKILL.md"))
		require.NoError(t, err, "non-argus Claude skill should be preserved after teardown")

		for _, skillPath := range lifecycle.SkillPaths() {
			_, err = os.Stat(filepath.Join(skillPath, "argus-doctor"))
			assert.True(t, os.IsNotExist(err), "%s/argus-doctor should be removed after teardown", skillPath)
		}
		for _, skillPath := range lifecycle.GlobalSkillPaths() {
			_, err = os.Stat(filepath.Join(skillPath, "argus-runtime", "SKILL.md"))
			require.NoError(t, err, "%s/argus-runtime/SKILL.md should survive project teardown", skillPath)
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

		_, cmdErr := executeSetupCmd(t, "--yes")
		require.NoError(t, cmdErr)

		settingsData, err := os.ReadFile(filepath.Join(".claude", "settings.json"))
		require.NoError(t, err)

		var settings map[string]any
		require.NoError(t, json.Unmarshal(settingsData, &settings))

		hooks := mustJSONObject(t, settings["hooks"])

		entries := mustJSONArray(t, hooks["UserPromptSubmit"])

		settingsStr := string(settingsData)
		assert.Contains(t, settingsStr, "my-other-tool", "non-argus hook should be present after setup")
		assert.Contains(t, settingsStr, "argus tick", "argus hook should be present after setup")
		assert.GreaterOrEqual(t, len(entries), 2, "should have at least 2 hook entries (non-argus + argus)")

		_, cmdErr = executeTeardownCmd(t, "--yes")
		require.NoError(t, cmdErr)

		settingsData, err = os.ReadFile(filepath.Join(".claude", "settings.json"))
		require.NoError(t, err)

		require.NoError(t, json.Unmarshal(settingsData, &settings))

		mustJSONObject(t, settings["hooks"])

		settingsStr = string(settingsData)
		assert.Contains(t, settingsStr, "my-other-tool", "non-argus hook should be preserved after teardown")
		assert.NotContains(t, settingsStr, "argus tick", "argus tick hook should be removed after teardown")
		assert.NotContains(t, settingsStr, "argus trap", "argus trap hook should be removed after teardown")
	})
}
