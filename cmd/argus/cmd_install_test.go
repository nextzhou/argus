package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/install"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfirmSubdirectoryInstall(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"y confirms", "y\n", true},
		{"n declines", "n\n", false},
		{"yes confirms", "yes\n", true},
		{"Yes confirms", "Yes\n", true},
		{"empty input declines", "\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)

			got, err := confirmSubdirectoryInstall(cmd, "/fake/root", strings.NewReader(tt.input), true)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfirmSubdirectoryInstall_NonTTY(t *testing.T) {
	cmd := &cobra.Command{}
	got, err := confirmSubdirectoryInstall(cmd, "/fake/root", strings.NewReader("y\n"), false)

	assert.False(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
}

func TestConfirmWorkspaceInstall(t *testing.T) {
	t.Run("registers new workspace", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		got, err := confirmWorkspaceInstall(cmd, "~/work/company", false, strings.NewReader("yes\n"), true)

		require.NoError(t, err)
		assert.True(t, got)
		assert.Contains(t, buf.String(), "This will register the workspace path:")
		assert.Contains(t, buf.String(), "~/work/company")
		assert.Contains(t, buf.String(), "global hooks and global skills")
	})

	t.Run("refreshes existing workspace", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		got, err := confirmWorkspaceInstall(cmd, "~/work/company", true, strings.NewReader("yes\n"), true)

		require.NoError(t, err)
		assert.True(t, got)
		assert.Contains(t, buf.String(), "This workspace path is already registered:")
		assert.Contains(t, buf.String(), "refresh global hooks, global skills, and global artifacts")
	})
}

func TestConfirmWorkspaceInstall_NonTTY(t *testing.T) {
	cmd := &cobra.Command{}
	got, err := confirmWorkspaceInstall(cmd, "~/work/company", false, strings.NewReader("yes\n"), false)

	assert.False(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
}

func TestConfirmWorkspaceUninstall(t *testing.T) {
	t.Run("non-final workspace", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		got, err := confirmWorkspaceUninstall(cmd, "~/work/company", false, strings.NewReader("y\n"), true)

		require.NoError(t, err)
		assert.True(t, got)
		assert.Contains(t, buf.String(), "This will unregister the workspace path:")
		assert.Contains(t, buf.String(), "stop guiding projects inside this workspace")
		assert.NotContains(t, buf.String(), "No registered workspaces will remain.")
	})

	t.Run("last workspace", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		got, err := confirmWorkspaceUninstall(cmd, "~/work/company", true, strings.NewReader("y\n"), true)

		require.NoError(t, err)
		assert.True(t, got)
		assert.Contains(t, buf.String(), "No registered workspaces will remain.")
		assert.Contains(t, buf.String(), "remove global hooks and global skills")
	})
}

func TestConfirmWorkspaceUninstall_NonTTY(t *testing.T) {
	cmd := &cobra.Command{}
	got, err := confirmWorkspaceUninstall(cmd, "~/work/company", true, strings.NewReader("yes\n"), false)

	assert.False(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
}

func TestUninstallNoArgusDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, os.MkdirAll(".git", 0o700))

	output, cmdErr := executeUninstallCmd(t, "--yes")

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no Argus installation found")

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	assert.Equal(t, "error", data["status"])
}

func TestUninstallPreservesNonArgusSkills(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	initGitRepo(t)

	_, cmdErr := executeInstallCmd(t, "--yes")
	require.NoError(t, cmdErr)

	for _, skillPath := range install.SkillPaths() {
		customDir := filepath.Join(skillPath, "my-team-skill")
		require.NoError(t, os.MkdirAll(customDir, 0o700))
		require.NoError(t, os.WriteFile(
			filepath.Join(customDir, "SKILL.md"),
			[]byte("# My Team Skill\n"),
			0o600,
		))
	}

	_, cmdErr = executeUninstallCmd(t, "--yes")
	require.NoError(t, cmdErr)

	for _, skillPath := range install.SkillPaths() {
		_, err := os.Stat(filepath.Join(skillPath, "my-team-skill", "SKILL.md"))
		require.NoError(t, err, "%s/my-team-skill/SKILL.md should survive uninstall", skillPath)

		_, err = os.Stat(filepath.Join(skillPath, "argus-doctor"))
		assert.True(t, os.IsNotExist(err), "%s/argus-doctor should be removed", skillPath)
	}
}

func TestUninstallNonInteractiveWithoutYes(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	initGitRepo(t)

	_, cmdErr := executeInstallCmd(t, "--yes")
	require.NoError(t, cmdErr)

	output, cmdErr := executeUninstallCmdWithInput(t, bytes.NewBuffer(nil))

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "--yes")

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	assert.Equal(t, "error", data["status"])
	assert.Contains(t, data["message"], "use --yes")

	_, err := os.Stat(".argus")
	assert.NoError(t, err, ".argus/ should still exist after refused uninstall")
}
