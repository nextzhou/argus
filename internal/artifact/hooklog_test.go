package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHookLogStoreAppend(t *testing.T) {
	t.Run("project file-backed store appends hook entries", func(t *testing.T) {
		projectRoot := t.TempDir()
		store := NewProjectSet(projectRoot).HookLog()

		require.NoError(t, store.Append("tick", true, "pipeline: build"))

		logPath := filepath.Join(projectRoot, ".argus", "logs", "hook.log")
		//nolint:gosec // logPath is constructed from the temp project root in this test.
		content, err := os.ReadFile(logPath)
		require.NoError(t, err)

		line := string(content)
		assert.Contains(t, line, "[tick]")
		assert.Contains(t, line, "OK")
		assert.Contains(t, line, "pipeline: build")
	})

	t.Run("global fallback store writes into user config root", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)

		store, err := NewFallbackHookLogStore()
		require.NoError(t, err)
		require.NoError(t, store.Append("trap", false, "timeout"))

		logPath := filepath.Join(homeDir, ".config", "argus", "logs", "hook.log")
		//nolint:gosec // logPath is constructed from the temp HOME in this test.
		content, err := os.ReadFile(logPath)
		require.NoError(t, err)

		line := strings.TrimSpace(string(content))
		assert.Contains(t, line, "[trap]")
		assert.Contains(t, line, "ERROR")
		assert.Contains(t, line, "timeout")
	})
}
