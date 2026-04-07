package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogHookExecution_ProjectPath(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := tempDir

	err := LogHookExecution(projectRoot, "tick", true, "pipeline: release")
	require.NoError(t, err)

	logPath := filepath.Join(projectRoot, ".argus", "logs", "hook.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logLine := string(content)
	assert.Contains(t, logLine, "[tick]")
	assert.Contains(t, logLine, "OK")
	assert.Contains(t, logLine, "pipeline: release")
	assert.True(t, strings.HasSuffix(logLine, "\n"))
}

func TestLogHookExecution_Fallback(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	err := LogHookExecution("", "trap", false, "error: timeout")
	require.NoError(t, err)

	logPath := filepath.Join(tempDir, ".config", "argus", "logs", "hook.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logLine := string(content)
	assert.Contains(t, logLine, "[trap]")
	assert.Contains(t, logLine, "ERROR")
	assert.Contains(t, logLine, "error: timeout")
}

func TestLogHookExecution_AutoCreateDirs(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := tempDir

	// Verify .argus/logs doesn't exist yet
	logsDir := filepath.Join(projectRoot, ".argus", "logs")
	assert.NoDirExists(t, logsDir)

	err := LogHookExecution(projectRoot, "tick", true, "test")
	require.NoError(t, err)

	// Verify directories were created
	assert.DirExists(t, logsDir)
	logPath := filepath.Join(logsDir, "hook.log")
	assert.FileExists(t, logPath)
}

func TestLogHookExecution_AppendMode(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := tempDir

	// Write first entry
	err := LogHookExecution(projectRoot, "tick", true, "first entry")
	require.NoError(t, err)

	// Write second entry
	err = LogHookExecution(projectRoot, "trap", false, "second entry")
	require.NoError(t, err)

	logPath := filepath.Join(projectRoot, ".argus", "logs", "hook.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logContent := string(content)
	lines := strings.Split(strings.TrimSuffix(logContent, "\n"), "\n")
	assert.Len(t, lines, 2)
	assert.Contains(t, lines[0], "tick")
	assert.Contains(t, lines[0], "first entry")
	assert.Contains(t, lines[1], "trap")
	assert.Contains(t, lines[1], "second entry")
}

func TestLogHookExecution_ErrorStatus(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := tempDir

	err := LogHookExecution(projectRoot, "tick", false, "execution failed")
	require.NoError(t, err)

	logPath := filepath.Join(projectRoot, ".argus", "logs", "hook.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logLine := string(content)
	assert.Contains(t, logLine, "ERROR")
	assert.NotContains(t, logLine, "OK")
}
