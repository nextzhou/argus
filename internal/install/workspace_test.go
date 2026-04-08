package install

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nextzhou/argus/internal/assets"
	workspacecfg "github.com/nextzhou/argus/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspace(t *testing.T) {
	t.Run("InstallWorkspace_Success", TestInstallWorkspace_Success)
	t.Run("InstallWorkspace_DuplicateRegistration", TestInstallWorkspace_DuplicateRegistration)
	t.Run("InstallWorkspace_NonExistentPath", TestInstallWorkspace_NonExistentPath)
	t.Run("InstallWorkspace_NotDirectory", TestInstallWorkspace_NotDirectory)
	t.Run("InstallWorkspace_NestedPathsAreStoredSeparately", TestInstallWorkspace_NestedPathsAreStoredSeparately)
	t.Run("InstallGlobalHooks_ClaudeCode", TestInstallGlobalHooks_ClaudeCode)
	t.Run("InstallGlobalHooks_Codex", TestInstallGlobalHooks_Codex)
	t.Run("InstallGlobalHooks_OpenCode", TestInstallGlobalHooks_OpenCode)
	t.Run("InstallGlobalSkills", TestInstallGlobalSkills)
	t.Run("GlobalSkillNames", TestGlobalSkillNames)
	t.Run("GlobalSkillPaths", TestGlobalSkillPaths)
	t.Run("UserConfigPath", TestUserConfigPath)
}

func TestInstallWorkspace_Success(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work", "company")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	require.NoError(t, InstallWorkspace(workspaceDir+string(filepath.Separator)))

	config, err := workspacecfg.LoadConfig(UserConfigPath())
	require.NoError(t, err)
	assert.Equal(t, []string{"~/work/company"}, config.Workspaces)

	settings := readJSONFile(t, filepath.Join(homeDir, claudeSettingsRelativePath))
	assert.Equal(t, []string{"argus tick --agent claude-code --global"}, hookCommandsForEvent(t, settings, "UserPromptSubmit"))
	assert.Equal(t, []string{"argus trap --agent claude-code --global"}, hookCommandsForEvent(t, settings, "PreToolUse"))

	codexHooks := readJSONFile(t, filepath.Join(homeDir, codexHooksRelativePath))
	assert.Equal(t, []string{"argus tick --agent codex --global"}, hookCommandsForEvent(t, codexHooks, "UserPromptSubmit"))
	assert.Equal(t, []string{"argus trap --agent codex --global"}, hookCommandsForEvent(t, codexHooks, "PreToolUse"))

	opencodePlugin, err := os.ReadFile(filepath.Join(homeDir, ".config", "opencode", "plugins", "argus.ts"))
	require.NoError(t, err)
	assert.Contains(t, string(opencodePlugin), "argus tick --agent opencode --global")
	assert.Contains(t, string(opencodePlugin), "argus trap --agent opencode --global")

	assertGlobalSkillsReleased(t)
}

func TestInstallWorkspace_DuplicateRegistration(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work", "company")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))
	require.NoError(t, InstallWorkspace(workspaceDir))

	warning, err := captureStderr(t, func() error {
		return InstallWorkspace(filepath.Join(workspaceDir, "."))
	})
	require.NoError(t, err)
	assert.Contains(t, warning, "workspace already registered")
	assert.Contains(t, warning, "~/work/company")

	config, err := workspacecfg.LoadConfig(UserConfigPath())
	require.NoError(t, err)
	assert.Equal(t, []string{"~/work/company"}, config.Workspaces)
}

func TestInstallWorkspace_NonExistentPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := InstallWorkspace(filepath.Join(os.Getenv("HOME"), "missing"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace path does not exist")
}

func TestInstallWorkspace_NotDirectory(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	filePath := filepath.Join(homeDir, "not-a-directory")
	require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o644))

	err := InstallWorkspace(filePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace path is not a directory")
}

func TestInstallWorkspace_NestedPathsAreStoredSeparately(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	parentDir := filepath.Join(homeDir, "work")
	nestedDir := filepath.Join(parentDir, "client-x")
	require.NoError(t, os.MkdirAll(nestedDir, 0o755))

	require.NoError(t, InstallWorkspace(parentDir))
	require.NoError(t, InstallWorkspace(nestedDir))

	config, err := workspacecfg.LoadConfig(UserConfigPath())
	require.NoError(t, err)
	assert.Equal(t, []string{"~/work", "~/work/client-x"}, config.Workspaces)
}

func TestInstallGlobalHooks_ClaudeCode(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, InstallGlobalHooks([]string{agentClaudeCode}))

	settings := readJSONFile(t, filepath.Join(homeDir, claudeSettingsRelativePath))
	assert.Equal(t, []string{"argus tick --agent claude-code --global"}, hookCommandsForEvent(t, settings, "UserPromptSubmit"))
	assert.Equal(t, []string{"argus trap --agent claude-code --global"}, hookCommandsForEvent(t, settings, "PreToolUse"))
}

func TestInstallGlobalHooks_Codex(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, InstallGlobalHooks([]string{agentCodex}))

	hooks := readJSONFile(t, filepath.Join(homeDir, codexHooksRelativePath))
	assert.Equal(t, []string{"argus tick --agent codex --global"}, hookCommandsForEvent(t, hooks, "UserPromptSubmit"))
	assert.Equal(t, []string{"argus trap --agent codex --global"}, hookCommandsForEvent(t, hooks, "PreToolUse"))

	config := readTOMLFile(t, filepath.Join(homeDir, codexConfigRelativePath))
	assert.Equal(t, true, config["codex_hooks"])
}

func TestInstallGlobalHooks_OpenCode(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, InstallGlobalHooks([]string{agentOpenCode}))

	pluginPath := filepath.Join(homeDir, ".config", "opencode", "plugins", "argus.ts")
	plugin, err := os.ReadFile(pluginPath)
	require.NoError(t, err)
	assert.Contains(t, string(plugin), "argus tick --agent opencode --global")
	assert.Contains(t, string(plugin), "argus trap --agent opencode --global")
}

func TestInstallGlobalSkills(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, InstallGlobalSkills())

	assertGlobalSkillsReleased(t)

	for _, skillPath := range GlobalSkillPaths() {
		_, err := os.Stat(filepath.Join(skillPath, "argus-status", "SKILL.md"))
		assert.True(t, os.IsNotExist(err))
	}

	assert.Equal(t, 9, countSkillMarkdownFiles(t, GlobalSkillPaths()))
}

func TestGlobalSkillNames(t *testing.T) {
	assert.True(t, slices.Equal(
		[]string{"argus-install", "argus-uninstall", "argus-doctor"},
		GlobalSkillNames(),
	))
}

func TestGlobalSkillPaths(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	assert.True(t, slices.Equal([]string{
		filepath.Join(homeDir, ".claude", "skills"),
		filepath.Join(homeDir, ".agents", "skills"),
		filepath.Join(homeDir, ".config", "opencode", "skills"),
	}, GlobalSkillPaths()))
}

func TestUserConfigPath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	assert.Equal(t, filepath.Join(homeDir, ".config", "argus", "config.yaml"), UserConfigPath())
}

func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	fnErr := fn()

	require.NoError(t, w.Close())
	os.Stderr = oldStderr

	output, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return string(output), fnErr
}

func assertGlobalSkillsReleased(t *testing.T) {
	t.Helper()

	for _, skillName := range GlobalSkillNames() {
		want, err := assets.ReadAsset(filepath.Join("skills", skillName, "SKILL.md"))
		require.NoError(t, err)

		for _, skillPath := range GlobalSkillPaths() {
			got, err := os.ReadFile(filepath.Join(skillPath, skillName, "SKILL.md"))
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))
		}
	}
}

func countSkillMarkdownFiles(t *testing.T, roots []string) int {
	t.Helper()

	count := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && filepath.Base(path) == "SKILL.md" {
				count++
			}
			return nil
		})
		require.NoError(t, err)
	}

	return count
}
