package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeCodeHookTemplate(t *testing.T) {
	rendered, err := RenderHookTemplate("claude-code", false)
	require.NoError(t, err)

	// Must be valid JSON
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(rendered, &parsed), "rendered output must be valid JSON")

	content := string(rendered)
	assert.Contains(t, content, "command -v argus")
	assert.Contains(t, content, expectedMissingArgusHookMessage())
	assert.Contains(t, content, "argus tick --agent claude-code")
	assert.NotContains(t, content, "argus trap --agent claude-code")
	assert.NotContains(t, content, "--global")
}

func TestCodexHookTemplate(t *testing.T) {
	rendered, err := RenderHookTemplate("codex", false)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(rendered, &parsed))

	content := string(rendered)
	assert.Contains(t, content, "command -v argus")
	assert.Contains(t, content, expectedMissingArgusHookMessage())
	assert.Contains(t, content, "argus tick --agent codex")
	assert.NotContains(t, content, "argus trap --agent codex")
	assert.NotContains(t, content, `"matcher": "Bash"`)
}

func TestOpenCodePluginTemplate(t *testing.T) {
	rendered, err := RenderHookTemplate("opencode", false)
	require.NoError(t, err)

	content := string(rendered)
	assert.Contains(t, content, `import type { Plugin } from "@opencode-ai/plugin"`)
	assert.Contains(t, content, "export const ArgusPlugin: Plugin = async")
	assert.Contains(t, content, "const pendingInjections = new Map<string, string>()")
	assert.Contains(t, content, "argus tick --agent opencode")
	assert.NotContains(t, content, "argus trap --agent opencode")
	assert.Contains(t, content, "chat.message")
	assert.Contains(t, content, "experimental.chat.messages.transform")
	assert.NotContains(t, content, "tool.execute.before")
	assert.Contains(t, content, "which argus")
	assert.Contains(t, content, "client.session.get({")
	assert.Contains(t, content, "path: { id: input.sessionID }")
	assert.Contains(t, content, "parentID: session.data?.parentID")
	assert.Contains(t, content, "cwd: directory")
	assert.Contains(t, content, ".cwd(directory)")
	assert.Contains(t, content, "pendingInjections.set(input.sessionID, text)")
	assert.Contains(t, content, "lastUserMessage.parts.splice(textPartIndex, 0, {")
	assert.Contains(t, content, "sessionID,")
	assert.Contains(t, content, "synthetic: true")
	assert.NotContains(t, content, "export default plugin")
}

func TestHookTemplateGlobalFlag(t *testing.T) {
	agents := []string{"claude-code", "codex", "opencode"}
	for _, agent := range agents {
		t.Run(agent, func(t *testing.T) {
			rendered, err := RenderHookTemplate(agent, true)
			require.NoError(t, err)

			content := string(rendered)
			assert.Contains(t, content, "--global")
		})
	}
}

func TestHookTemplateUnsupportedAgent(t *testing.T) {
	_, err := RenderHookTemplate("unknown-agent", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported agent")
}

func TestShellHookTemplateMissingBinaryFailsOpen(t *testing.T) {
	tests := []struct {
		name   string
		agent  string
		global bool
	}{
		{name: "claude-code local", agent: "claude-code"},
		{name: "codex global", agent: "codex", global: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := renderFirstHookCommand(t, tt.agent, tt.global)

			//nolint:gosec // command is rendered from the built-in hook template under test, not from user input.
			cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", command)
			cmd.Env = []string{"PATH=" + t.TempDir()}

			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "wrapper should fail open: %s", string(output))
			assert.Equal(t, expectedMissingArgusHookMessage()+"\n", string(output))
		})
	}
}

func TestShellHookTemplateExecutesArgusWhenPresent(t *testing.T) {
	tests := []struct {
		name     string
		agent    string
		global   bool
		wantArgs []string
	}{
		{
			name:     "claude-code local",
			agent:    "claude-code",
			wantArgs: []string{"tick", "--agent", "claude-code"},
		},
		{
			name:     "codex global",
			agent:    "codex",
			global:   true,
			wantArgs: []string{"tick", "--agent", "codex", "--global"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := renderFirstHookCommand(t, tt.agent, tt.global)
			binDir := t.TempDir()
			argsPath := filepath.Join(binDir, "args.txt")
			stubPath := filepath.Join(binDir, "argus")

			//nolint:gosec // The test stub must be executable and is written under a temp directory controlled by the test.
			require.NoError(t, os.WriteFile(stubPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \""+argsPath+"\"\n"), 0o700))

			//nolint:gosec // command is rendered from the built-in hook template under test, not from user input.
			cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", command)
			cmd.Env = []string{"PATH=" + binDir}

			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "wrapper should execute argus: %s", string(output))
			assert.Empty(t, string(output))

			//nolint:gosec // argsPath points to test-generated output under the temp directory controlled by the test.
			data, err := os.ReadFile(argsPath)
			require.NoError(t, err)
			assert.Equal(t, tt.wantArgs, strings.Split(strings.TrimSpace(string(data)), "\n"))
		})
	}
}

func TestOpenCodePluginBiome(t *testing.T) {
	if os.Getenv("ARGUS_EXTERNAL_TOOLS") == "" {
		t.Skip("set ARGUS_EXTERNAL_TOOLS=1 to enable external tool validation")
	}

	// Skip if biome is not installed
	if _, err := exec.LookPath("biome"); err != nil {
		t.Skip("biome not installed, skipping TypeScript validation")
	}

	rendered, err := RenderHookTemplate("opencode", false)
	require.NoError(t, err)

	// Write to temp file
	dir := t.TempDir()
	tsFile := filepath.Join(dir, "argus.ts")
	require.NoError(t, os.WriteFile(tsFile, rendered, 0o600))

	// Run biome check
	//nolint:gosec // This test executes the local biome binary against a temp file under test control.
	cmd := exec.CommandContext(context.Background(), "biome", "check", tsFile)
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "biome check failed: %s", string(output))
}

func TestClaudeCodeTemplateLocalMode(t *testing.T) {
	rendered, err := RenderHookTemplate("claude-code", false)
	require.NoError(t, err)
	assert.NotContains(t, string(rendered), "--global")
}

func renderFirstHookCommand(t *testing.T, agent string, global bool) string {
	t.Helper()

	rendered, err := RenderHookTemplate(agent, global)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(rendered, &parsed))

	commands := hookCommandsForEvent(t, parsed, "UserPromptSubmit")
	require.Len(t, commands, 1)
	return commands[0]
}
