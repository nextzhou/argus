package install

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeCodeHookInstallUninstall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := newTestProjectRoot(t)

	require.NoError(t, InstallHooks(projectRoot, []string{"claude-code"}))

	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")
	settings := readJSONFile(t, settingsPath)

	assert.Equal(t, []string{"argus tick --agent claude-code"}, hookCommandsForEvent(t, settings, "UserPromptSubmit"))
	assert.Empty(t, hookCommandsForEvent(t, settings, "PreToolUse"))

	require.NoError(t, UninstallHooks(projectRoot, []string{"claude-code"}))

	settings = readJSONFile(t, settingsPath)
	assert.NotContains(t, settings, "hooks")
	assert.Equal(t, map[string]any{}, settings)
}

func TestClaudeCodePreserveNonArgusHooks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := newTestProjectRoot(t)
	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")

	writeTestFile(t, settingsPath, `{
	  "permissions": {
	    "allow": ["Read"]
	  },
	  "hooks": {
	    "UserPromptSubmit": [
	      {
	        "hooks": [
	          {
	            "type": "command",
	            "command": "custom <hook>",
	            "timeout": 5,
	            "statusMessage": "Custom"
	          },
	          {
	            "type": "command",
	            "command": "/tmp/bin/argus tick --agent claude-code",
	            "timeout": 5,
	            "statusMessage": "Old Argus"
	          }
	        ]
	      }
	    ],
	    "PreToolUse": [
	      {
	        "hooks": [
	          {
	            "type": "command",
	            "command": "custom pre-tool",
	            "timeout": 5,
	            "statusMessage": "Custom PreTool"
	          },
	          {
	            "type": "command",
	            "command": "/tmp/bin/argus trap --agent claude-code",
	            "timeout": 5,
	            "statusMessage": "Old Argus Trap"
	          }
	        ]
	      }
	    ]
	  }
	}`)

	require.NoError(t, InstallHooks(projectRoot, []string{"claude-code"}))

	//nolint:gosec // The test reads the settings file it just asked InstallHooks to create.
	rawSettings, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(rawSettings), "custom <hook>")
	assert.NotContains(t, string(rawSettings), `\u003c`)

	settings := readJSONFile(t, settingsPath)
	assert.Equal(t, []string{"custom <hook>", "argus tick --agent claude-code"}, hookCommandsForEvent(t, settings, "UserPromptSubmit"))
	assert.Equal(t, []string{"custom pre-tool"}, hookCommandsForEvent(t, settings, "PreToolUse"))
	assert.Equal(t, map[string]any{"allow": []any{"Read"}}, settings["permissions"])

	require.NoError(t, UninstallHooks(projectRoot, []string{"claude-code"}))

	settings = readJSONFile(t, settingsPath)
	assert.Equal(t, []string{"custom <hook>"}, hookCommandsForEvent(t, settings, "UserPromptSubmit"))
	assert.Equal(t, []string{"custom pre-tool"}, hookCommandsForEvent(t, settings, "PreToolUse"))
	assert.Equal(t, map[string]any{"allow": []any{"Read"}}, settings["permissions"])
}

func TestIdempotentInstall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := newTestProjectRoot(t)

	require.NoError(t, InstallHooks(projectRoot, []string{"claude-code"}))
	require.NoError(t, InstallHooks(projectRoot, []string{"claude-code"}))

	settings := readJSONFile(t, filepath.Join(projectRoot, ".claude", "settings.json"))
	assert.Equal(t, []string{"argus tick --agent claude-code"}, hookCommandsForEvent(t, settings, "UserPromptSubmit"))
	assert.Empty(t, hookCommandsForEvent(t, settings, "PreToolUse"))
}

func TestClaudeCodeSettingsNotExists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := newTestProjectRoot(t)

	require.NoError(t, InstallHooks(projectRoot, []string{"claude-code"}))

	_, err := os.Stat(filepath.Join(projectRoot, ".claude", "settings.json"))
	assert.NoError(t, err)
}

func TestClaudeCodeSettingsInvalidJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := newTestProjectRoot(t)
	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")

	writeTestFile(t, settingsPath, `{invalid json`)

	err := InstallHooks(projectRoot, []string{"claude-code"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing claude code settings")
}

func TestCodexConfigToml(t *testing.T) {
	t.Run("creates config when missing", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		projectRoot := newTestProjectRoot(t)

		require.NoError(t, InstallHooks(projectRoot, []string{"codex"}))

		config := readTOMLFile(t, filepath.Join(homeDir, ".codex", "config.toml"))
		assert.Equal(t, true, config["codex_hooks"])
	})

	t.Run("preserves existing fields", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		projectRoot := newTestProjectRoot(t)
		configPath := filepath.Join(homeDir, ".codex", "config.toml")

		writeTestFile(t, configPath, "theme = \"dark\"\n[nested]\nvalue = 1\n")

		require.NoError(t, InstallHooks(projectRoot, []string{"codex"}))

		config := readTOMLFile(t, configPath)
		assert.Equal(t, true, config["codex_hooks"])
		assert.Equal(t, "dark", config["theme"])
		assert.Equal(t, map[string]any{"value": int64(1)}, config["nested"])
	})
}

func TestCodexHooksJson(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := newTestProjectRoot(t)

	require.NoError(t, InstallHooks(projectRoot, []string{"codex"}))

	hooksPath := filepath.Join(projectRoot, ".codex", "hooks.json")
	hooks := readJSONFile(t, hooksPath)
	assert.Equal(t, []string{"argus tick --agent codex"}, hookCommandsForEvent(t, hooks, "UserPromptSubmit"))
	assert.Empty(t, hookCommandsForEvent(t, hooks, "PreToolUse"))

	require.NoError(t, UninstallHooks(projectRoot, []string{"codex"}))

	_, err := os.Stat(hooksPath)
	require.ErrorIs(t, err, os.ErrNotExist)

	config := readTOMLFile(t, filepath.Join(os.Getenv("HOME"), ".codex", "config.toml"))
	assert.Equal(t, true, config["codex_hooks"])
}

func TestOpenCodePlugin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := newTestProjectRoot(t)

	require.NoError(t, InstallHooks(projectRoot, []string{"opencode"}))

	pluginPath := filepath.Join(projectRoot, ".opencode", "plugins", "argus.ts")
	//nolint:gosec // The test reads the plugin file it just asked InstallHooks to create.
	pluginContent, err := os.ReadFile(pluginPath)
	require.NoError(t, err)
	assert.Contains(t, string(pluginContent), "argus tick --agent opencode")
	assert.Contains(t, string(pluginContent), `import type { Plugin } from "@opencode-ai/plugin"`)
	assert.Contains(t, string(pluginContent), "export const ArgusPlugin: Plugin = async")
	assert.Contains(t, string(pluginContent), "parentID: session.data?.parentID")
	assert.Contains(t, string(pluginContent), "experimental.chat.messages.transform")
	assert.Contains(t, string(pluginContent), "synthetic: true")
	assert.NotContains(t, string(pluginContent), "argus trap --agent opencode")

	require.NoError(t, UninstallHooks(projectRoot, []string{"opencode"}))

	_, err = os.Stat(pluginPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestUninstallNotInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := newTestProjectRoot(t)

	assert.NoError(t, UninstallHooks(projectRoot, []string{"claude-code", "codex", "opencode"}))
}

func newTestProjectRoot(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(projectRoot, ".git"), 0o700))
	return projectRoot
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()

	//nolint:gosec // Test reads a file it created at a controlled path.
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(data))
	var parsed map[string]any
	require.NoError(t, decoder.Decode(&parsed))
	return parsed
}

func readTOMLFile(t *testing.T, path string) map[string]any {
	t.Helper()

	//nolint:gosec // Test reads a file it created at a controlled path.
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, toml.Unmarshal(data, &parsed))
	return parsed
}

func hookCommandsForEvent(t *testing.T, settings map[string]any, event string) []string {
	t.Helper()

	hooksMap, ok := settings["hooks"]
	if !ok {
		return nil
	}

	events := requireJSONMap(t, hooksMap)
	eventEntriesValue, ok := events[event]
	if !ok {
		return nil
	}

	eventEntries := requireJSONArray(t, eventEntriesValue)
	commands := make([]string, 0)
	for _, entryValue := range eventEntries {
		entry := requireJSONMap(t, entryValue)
		hooks := requireJSONArray(t, entry["hooks"])
		for _, hookValue := range hooks {
			hook := requireJSONMap(t, hookValue)
			command, ok := hook["command"].(string)
			require.True(t, ok)
			commands = append(commands, command)
		}
	}

	return commands
}

func requireJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()

	parsed, ok := value.(map[string]any)
	require.True(t, ok)
	return parsed
}

func requireJSONArray(t *testing.T, value any) []any {
	t.Helper()

	parsed, ok := value.([]any)
	require.True(t, ok)
	return parsed
}
