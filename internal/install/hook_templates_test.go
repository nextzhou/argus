package install

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
	assert.Contains(t, content, "argus tick --agent codex")
	assert.NotContains(t, content, "argus trap --agent codex")
	assert.NotContains(t, content, `"matcher": "Bash"`)
}

func TestOpenCodePluginTemplate(t *testing.T) {
	rendered, err := RenderHookTemplate("opencode", false)
	require.NoError(t, err)

	content := string(rendered)
	assert.Contains(t, content, "argus tick --agent opencode")
	assert.NotContains(t, content, "argus trap --agent opencode")
	assert.Contains(t, content, "chat.message")
	assert.NotContains(t, content, "tool.execute.before")
	assert.Contains(t, content, "which argus")
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

func TestOpenCodePluginBiome(t *testing.T) {
	// Skip if biome is not installed
	if _, err := exec.LookPath("biome"); err != nil {
		t.Skip("biome not installed, skipping TypeScript validation")
	}

	rendered, err := RenderHookTemplate("opencode", false)
	require.NoError(t, err)

	// Write to temp file
	dir := t.TempDir()
	tsFile := filepath.Join(dir, "argus.ts")
	require.NoError(t, os.WriteFile(tsFile, rendered, 0o644))

	// Run biome check
	cmd := exec.Command("biome", "check", tsFile)
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "biome check failed: %s", string(output))
}

func TestClaudeCodeTemplateLocalMode(t *testing.T) {
	rendered, err := RenderHookTemplate("claude-code", false)
	require.NoError(t, err)
	assert.NotContains(t, string(rendered), "--global")
}
