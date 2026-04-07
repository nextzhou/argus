package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSaveConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.yaml")

	original := &Config{
		Workspaces: []string{"~/work/company", "~/work/client-x"},
	}

	err := SaveConfig(path, original)
	require.NoError(t, err)

	_, err = os.Stat(path)
	require.NoError(t, err)

	loaded, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, original.Workspaces, loaded.Workspaces)
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening config file")
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	err := os.WriteFile(path, []byte("{{invalid yaml"), 0o644)
	require.NoError(t, err)

	_, err = LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config YAML")
}

func TestLoadConfig_UnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.yaml")
	content := "workspaces:\n  - ~/work\nunknown_field: oops\n"
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)

	_, err = LoadConfig(path)
	require.Error(t, err, "unknown fields should cause parse error")
}

func TestLoadConfig_EmptyWorkspaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	err := os.WriteFile(path, []byte("workspaces: []\n"), 0o644)
	require.NoError(t, err)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Empty(t, cfg.Workspaces)
}
