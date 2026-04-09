package install

import (
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
	t.Run("InstallWorkspace_GlobalArtifacts", TestInstallWorkspace_GlobalArtifacts)
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

	result, err := InstallWorkspaceWithReport(workspaceDir + string(filepath.Separator))
	require.NoError(t, err)
	assert.Equal(t, "~/work/company", result.Path)
	assert.Contains(t, result.Report.AffectedPaths, "~/.config/argus/config.yaml")
	assert.Contains(t, result.Report.Changes.Created, "~/.config/argus/config.yaml")

	config, err := workspacecfg.LoadConfig(UserConfigPath())
	require.NoError(t, err)
	assert.Equal(t, []string{"~/work/company"}, config.Workspaces)

	settings := readJSONFile(t, filepath.Join(homeDir, claudeSettingsRelativePath))
	assert.Equal(t, []string{"argus tick --agent claude-code --global"}, hookCommandsForEvent(t, settings, "UserPromptSubmit"))
	assert.Empty(t, hookCommandsForEvent(t, settings, "PreToolUse"))

	codexHooks := readJSONFile(t, filepath.Join(homeDir, codexHooksRelativePath))
	assert.Equal(t, []string{"argus tick --agent codex --global"}, hookCommandsForEvent(t, codexHooks, "UserPromptSubmit"))
	assert.Empty(t, hookCommandsForEvent(t, codexHooks, "PreToolUse"))

	opencodePlugin, err := os.ReadFile(filepath.Join(homeDir, ".config", "opencode", "plugins", "argus.ts"))
	require.NoError(t, err)
	assert.Contains(t, string(opencodePlugin), "argus tick --agent opencode --global")
	assert.Contains(t, string(opencodePlugin), `import type { Plugin } from "@opencode-ai/plugin"`)
	assert.Contains(t, string(opencodePlugin), "export const ArgusPlugin: Plugin = async")
	assert.Contains(t, string(opencodePlugin), "experimental.chat.messages.transform")
	assert.NotContains(t, string(opencodePlugin), "argus trap --agent opencode --global")

	assertGlobalSkillsReleased(t)
}

func TestInstallWorkspace_GlobalArtifacts(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work", "company")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	require.NoError(t, InstallWorkspace(workspaceDir))

	globalRoot := filepath.Join(homeDir, ".config", "argus")

	// Verify global directory structure exists.
	for _, dir := range []string{"invariants", "workflows", "pipelines", "logs"} {
		dirPath := filepath.Join(globalRoot, dir)
		info, err := os.Stat(dirPath)
		require.NoError(t, err, "directory %s should exist", dir)
		assert.True(t, info.IsDir(), "%s should be a directory", dir)
	}

	// Verify only global-specific invariant is released (not project-level ones)
	projectInitPath := filepath.Join(globalRoot, "invariants", "argus-project-init.yaml")
	_, err := os.Stat(projectInitPath)
	require.NoError(t, err, "argus-project-init.yaml should exist")

	data, err := os.ReadFile(projectInitPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "id: argus-project-init")
	assert.Contains(t, string(data), "test -d .argus")

	argusInitPath := filepath.Join(globalRoot, "invariants", "argus-init.yaml")
	_, err = os.Stat(argusInitPath)
	assert.True(t, os.IsNotExist(err), "argus-init.yaml should NOT exist in global scope")
}

func TestInstallWorkspace_DuplicateRegistration(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work", "company")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))
	require.NoError(t, InstallWorkspace(workspaceDir))

	result, err := InstallWorkspaceWithReport(filepath.Join(workspaceDir, "."))
	require.NoError(t, err)
	assert.True(t, result.AlreadyRegistered)
	assert.Empty(t, result.Report.Changes.Created)
	assert.Empty(t, result.Report.Changes.Updated)
	assert.Empty(t, result.Report.Changes.Removed)

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

func TestUninstallWorkspaceWithReport(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))
	require.NoError(t, InstallWorkspace(workspaceDir))

	result, err := UninstallWorkspaceWithReport(workspaceDir)
	require.NoError(t, err)
	assert.True(t, result.RemovedGlobalResource)
	assert.Contains(t, result.Report.AffectedPaths, "~/.config/argus/config.yaml")
	assert.Contains(t, result.Report.Changes.Removed, "~/.claude/skills/argus-*")
}

func TestInstallGlobalHooks_ClaudeCode(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, InstallGlobalHooks([]string{agentClaudeCode}))

	settings := readJSONFile(t, filepath.Join(homeDir, claudeSettingsRelativePath))
	assert.Equal(t, []string{"argus tick --agent claude-code --global"}, hookCommandsForEvent(t, settings, "UserPromptSubmit"))
	assert.Empty(t, hookCommandsForEvent(t, settings, "PreToolUse"))
}

func TestInstallGlobalHooks_Codex(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, InstallGlobalHooks([]string{agentCodex}))

	hooks := readJSONFile(t, filepath.Join(homeDir, codexHooksRelativePath))
	assert.Equal(t, []string{"argus tick --agent codex --global"}, hookCommandsForEvent(t, hooks, "UserPromptSubmit"))
	assert.Empty(t, hookCommandsForEvent(t, hooks, "PreToolUse"))

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
	assert.Contains(t, string(plugin), "parentID: session.data?.parentID")
	assert.Contains(t, string(plugin), "synthetic: true")
	assert.NotContains(t, string(plugin), "argus trap --agent opencode --global")
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

	assert.Equal(t, 12, countSkillMarkdownFiles(t, GlobalSkillPaths()))
}

func TestGlobalSkillNames(t *testing.T) {
	assert.True(t, slices.Equal(
		[]string{"argus-intro", "argus-install", "argus-uninstall", "argus-doctor"},
		GlobalSkillNames(),
	))
}

func TestInstallWorkspace_RefreshesGlobalResources(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work", "company")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))
	require.NoError(t, InstallWorkspace(workspaceDir))

	for _, skillPath := range GlobalSkillPaths() {
		require.NoError(t, os.MkdirAll(filepath.Join(skillPath, "argus-concepts"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skillPath, "argus-concepts", "SKILL.md"), []byte("# legacy\n"), 0o644))
	}

	introPath := filepath.Join(homeDir, ".agents", "skills", "argus-intro", "SKILL.md")
	require.NoError(t, os.WriteFile(introPath, []byte("# stale intro\n"), 0o644))

	result, err := InstallWorkspaceWithReport(workspaceDir)
	require.NoError(t, err)
	assert.True(t, result.AlreadyRegistered)
	assert.NotEmpty(t, result.Report.Changes.Updated)
	assert.NotEmpty(t, result.Report.Changes.Removed)

	for _, skillPath := range GlobalSkillPaths() {
		_, statErr := os.Stat(filepath.Join(skillPath, "argus-concepts"))
		assert.True(t, os.IsNotExist(statErr), "%s/argus-concepts should be pruned", skillPath)
		_, statErr = os.Stat(filepath.Join(skillPath, "argus-intro", "SKILL.md"))
		assert.NoError(t, statErr, "%s/argus-intro/SKILL.md should exist", skillPath)
	}
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
