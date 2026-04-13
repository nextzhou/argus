package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/lifecycle"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfirmProjectSetup(t *testing.T) {
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
		t.Run("git root/"+tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)

			got, err := confirmProjectSetup(cmd, "/fake/root", false, strings.NewReader(tt.input), true)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Contains(t, buf.String(), "This will set up project-level Argus in:")
			assert.Contains(t, buf.String(), "/fake/root")
			assert.Contains(t, buf.String(), "create or refresh .argus/")
			assert.Contains(t, buf.String(), "managed global skills for this user account")
		})

		t.Run("git subdirectory/"+tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)

			got, err := confirmProjectSetup(cmd, "/fake/root/subdir", true, strings.NewReader(tt.input), true)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Contains(t, buf.String(), "Current directory is not the Git root.")
			assert.Contains(t, buf.String(), "This will set up project-level Argus in this subdirectory:")
			assert.Contains(t, buf.String(), "/fake/root/subdir")
		})
	}
}

func TestConfirmProjectSetup_NonTTY(t *testing.T) {
	cmd := &cobra.Command{}
	got, err := confirmProjectSetup(cmd, "/fake/root", false, strings.NewReader("y\n"), false)

	assert.False(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
}

func TestConfirmWorkspaceSetup(t *testing.T) {
	t.Run("registers new workspace", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		got, err := confirmWorkspaceSetup(cmd, "~/work/company", false, strings.NewReader("yes\n"), true)

		require.NoError(t, err)
		assert.True(t, got)
		assert.Contains(t, buf.String(), "This will register the workspace path:")
		assert.Contains(t, buf.String(), "~/work/company")
		assert.Contains(t, buf.String(), "global hooks, global skills, and global artifacts")
	})

	t.Run("refreshes existing workspace", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		got, err := confirmWorkspaceSetup(cmd, "~/work/company", true, strings.NewReader("yes\n"), true)

		require.NoError(t, err)
		assert.True(t, got)
		assert.Contains(t, buf.String(), "This workspace path is already registered:")
		assert.Contains(t, buf.String(), "refresh global hooks, global skills, and global artifacts")
	})
}

func TestConfirmWorkspaceSetup_NonTTY(t *testing.T) {
	cmd := &cobra.Command{}
	got, err := confirmWorkspaceSetup(cmd, "~/work/company", false, strings.NewReader("yes\n"), false)

	assert.False(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
}

func TestConfirmWorkspaceTeardown(t *testing.T) {
	t.Run("non-final workspace", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		got, err := confirmWorkspaceTeardown(cmd, "~/work/company", false, strings.NewReader("y\n"), true)

		require.NoError(t, err)
		assert.True(t, got)
		assert.Contains(t, buf.String(), "This will unregister the workspace path:")
		assert.Contains(t, buf.String(), "stop guiding repositories inside this workspace")
		assert.NotContains(t, buf.String(), "No registered workspaces will remain.")
	})

	t.Run("last workspace", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		got, err := confirmWorkspaceTeardown(cmd, "~/work/company", true, strings.NewReader("y\n"), true)

		require.NoError(t, err)
		assert.True(t, got)
		assert.Contains(t, buf.String(), "No registered workspaces will remain.")
		assert.Contains(t, buf.String(), "remove global hooks, global skills, global artifacts, and the managed ~/.config/argus/ root")
	})
}

func TestConfirmWorkspaceTeardown_NonTTY(t *testing.T) {
	cmd := &cobra.Command{}
	got, err := confirmWorkspaceTeardown(cmd, "~/work/company", true, strings.NewReader("yes\n"), false)

	assert.False(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
}

func TestTeardownWithoutProjectSetup(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, os.MkdirAll(".git", 0o700))

	output, cmdErr := executeTeardownCmd(t, "--yes")

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no project-level Argus setup found")

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	assert.Equal(t, "error", data["status"])
}

func TestTeardownPreservesNonArgusSkills(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	initGitRepo(t)

	_, cmdErr := executeSetupCmd(t, "--yes")
	require.NoError(t, cmdErr)

	for _, skillPath := range lifecycle.SkillPaths() {
		customDir := filepath.Join(skillPath, "my-team-skill")
		require.NoError(t, os.MkdirAll(customDir, 0o700))
		require.NoError(t, os.WriteFile(
			filepath.Join(customDir, "SKILL.md"),
			[]byte("# My Team Skill\n"),
			0o600,
		))
	}

	_, cmdErr = executeTeardownCmd(t, "--yes")
	require.NoError(t, cmdErr)

	for _, skillPath := range lifecycle.SkillPaths() {
		_, err := os.Stat(filepath.Join(skillPath, "my-team-skill", "SKILL.md"))
		require.NoError(t, err, "%s/my-team-skill/SKILL.md should survive teardown", skillPath)

		_, err = os.Stat(filepath.Join(skillPath, "argus-doctor"))
		assert.True(t, os.IsNotExist(err), "%s/argus-doctor should be removed", skillPath)
	}
}

func TestTeardownJSONWithoutYes(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	initGitRepo(t)

	_, cmdErr := executeSetupCmd(t, "--yes")
	require.NoError(t, cmdErr)

	output, cmdErr := executeTeardownCmdWithInput(t, bytes.NewBuffer(nil))

	require.Error(t, cmdErr)
	assert.Equal(t, "project teardown requires --yes when --json is used; --json is non-interactive", cmdErr.Error())

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	assert.Equal(t, "error", data["status"])
	assert.Equal(t, "project teardown requires --yes when --json is used; --json is non-interactive", data["message"])

	_, err := os.Stat(".argus")
	assert.NoError(t, err, ".argus/ should still exist after refused teardown")
}

func TestSetupJSONWithoutYes(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	initGitRepo(t)

	output, cmdErr := executeSetupCmdWithInput(t, bytes.NewBuffer(nil))

	require.Error(t, cmdErr)
	assert.Equal(t, "project setup requires --yes when --json is used; --json is non-interactive", cmdErr.Error())

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	assert.Equal(t, "error", data["status"])
	assert.Equal(t, "project setup requires --yes when --json is used; --json is non-interactive", data["message"])

	_, err := os.Stat(".argus")
	assert.ErrorIs(t, err, os.ErrNotExist, ".argus/ should not be created after refused setup")
}

func TestSetupSubdirectoryJSONWithoutYes(t *testing.T) {
	repoRoot := t.TempDir()
	t.Chdir(repoRoot)
	t.Setenv("HOME", t.TempDir())
	initGitRepo(t)

	subdir := filepath.Join(repoRoot, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0o700))
	t.Chdir(subdir)

	output, cmdErr := executeSetupCmdWithInput(t, bytes.NewBuffer(nil))

	require.Error(t, cmdErr)
	assert.Equal(t, "project setup requires --yes when --json is used; --json is non-interactive", cmdErr.Error())

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	assert.Equal(t, "error", data["status"])
	assert.Equal(t, "project setup requires --yes when --json is used; --json is non-interactive", data["message"])

	_, err := os.Stat(filepath.Join(subdir, ".argus"))
	assert.ErrorIs(t, err, os.ErrNotExist, "subdirectory setup should not create .argus/ after refused setup")
}
