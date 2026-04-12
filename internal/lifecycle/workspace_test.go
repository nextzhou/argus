package lifecycle

import (
	"errors"
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
	t.Run("SetupWorkspace_Success", TestSetupWorkspace_Success)
	t.Run("SetupWorkspace_DuplicateRegistration", TestSetupWorkspace_DuplicateRegistration)
	t.Run("SetupWorkspace_NonExistentPath", TestSetupWorkspace_NonExistentPath)
	t.Run("SetupWorkspace_NotDirectory", TestSetupWorkspace_NotDirectory)
	t.Run("SetupWorkspace_NestedPathsAreStoredSeparately", TestSetupWorkspace_NestedPathsAreStoredSeparately)
	t.Run("SetupWorkspace_GlobalArtifacts", TestSetupWorkspace_GlobalArtifacts)
	t.Run("SetupGlobalHooks_ClaudeCode", TestSetupGlobalHooks_ClaudeCode)
	t.Run("SetupGlobalHooks_Codex", TestSetupGlobalHooks_Codex)
	t.Run("SetupGlobalHooks_OpenCode", TestSetupGlobalHooks_OpenCode)
	t.Run("SetupGlobalSkills", TestSetupGlobalSkills)
	t.Run("GlobalSkillNames", TestGlobalSkillNames)
	t.Run("GlobalSkillPaths", TestGlobalSkillPaths)
	t.Run("UserConfigPath", TestUserConfigPath)
}

func TestSetupWorkspace_Success(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work", "company")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))

	result, err := SetupWorkspaceWithReport(workspaceDir + string(filepath.Separator))
	require.NoError(t, err)
	assert.Contains(t, result.Report.AffectedPaths, "~/.config/argus/{invariants,workflows,pipelines,logs}/")
	assert.Equal(t, "~/work/company", result.Path)
	assert.Contains(t, result.Report.AffectedPaths, "~/.config/argus/config.yaml")
	assert.Contains(t, result.Report.Changes.Created, "~/.config/argus/config.yaml")

	config, err := workspacecfg.LoadConfig(UserConfigPath())
	require.NoError(t, err)
	assert.Equal(t, []string{"~/work/company"}, config.Workspaces)

	settings := readJSONFile(t, filepath.Join(homeDir, claudeSettingsRelativePath))
	claudeCommands := hookCommandsForEvent(t, settings, "UserPromptSubmit")
	require.Len(t, claudeCommands, 1)
	assertArgusShellHookCommand(t, claudeCommands[0], "claude-code", true)
	assert.Empty(t, hookCommandsForEvent(t, settings, "PreToolUse"))

	codexHooks := readJSONFile(t, filepath.Join(homeDir, codexHooksRelativePath))
	codexCommands := hookCommandsForEvent(t, codexHooks, "UserPromptSubmit")
	require.Len(t, codexCommands, 1)
	assertArgusShellHookCommand(t, codexCommands[0], "codex", true)
	assert.Empty(t, hookCommandsForEvent(t, codexHooks, "PreToolUse"))

	//nolint:gosec // Test reads a plugin file created under its temp HOME directory.
	opencodePlugin, err := os.ReadFile(filepath.Join(homeDir, ".config", "opencode", "plugins", "argus.ts"))
	require.NoError(t, err)
	assert.Contains(t, string(opencodePlugin), "argus tick --agent opencode --global")
	assert.Contains(t, string(opencodePlugin), `import type { Plugin } from "@opencode-ai/plugin"`)
	assert.Contains(t, string(opencodePlugin), "export const ArgusPlugin: Plugin = async")
	assert.Contains(t, string(opencodePlugin), "experimental.chat.messages.transform")
	assert.NotContains(t, string(opencodePlugin), "argus trap --agent opencode --global")

	assertGlobalSkillsReleased(t)
}

func TestSetupWorkspace_GlobalArtifacts(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work", "company")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))

	require.NoError(t, SetupWorkspace(workspaceDir))

	globalRoot := filepath.Join(homeDir, ".config", "argus")

	// Verify global directory structure exists.
	for _, dir := range []string{"invariants", "workflows", "pipelines", "logs"} {
		dirPath := filepath.Join(globalRoot, dir)
		info, err := os.Stat(dirPath)
		require.NoError(t, err, "directory %s should exist", dir)
		assert.True(t, info.IsDir(), "%s should be a directory", dir)
	}

	// Verify only global-specific invariant is released (not project-level ones)
	projectInitPath := filepath.Join(globalRoot, "invariants", "argus-project-setup.yaml")
	_, err := os.Stat(projectInitPath)
	require.NoError(t, err, "argus-project-setup.yaml should exist")

	//nolint:gosec // Test reads an invariant file created under its temp HOME directory.
	data, err := os.ReadFile(projectInitPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "id: argus-project-setup")
	assert.Contains(t, string(data), "test -d .argus")

	argusInitPath := filepath.Join(globalRoot, "invariants", "argus-project-init.yaml")
	_, err = os.Stat(argusInitPath)
	assert.True(t, os.IsNotExist(err), "argus-project-init.yaml should NOT exist in global scope")
}

func TestSetupWorkspace_DuplicateRegistration(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work", "company")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	require.NoError(t, SetupWorkspace(workspaceDir))

	result, err := SetupWorkspaceWithReport(filepath.Join(workspaceDir, "."))
	require.NoError(t, err)
	assert.True(t, result.AlreadyRegistered)
	assert.Contains(t, result.Report.AffectedPaths, "~/.config/argus/{invariants,workflows,pipelines,logs}/")
	assert.Empty(t, result.Report.Changes.Created)
	assert.Empty(t, result.Report.Changes.Updated)
	assert.Empty(t, result.Report.Changes.Removed)

	config, err := workspacecfg.LoadConfig(UserConfigPath())
	require.NoError(t, err)
	assert.Equal(t, []string{"~/work/company"}, config.Workspaces)
}

func TestSetupWorkspace_NonExistentPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := SetupWorkspace(filepath.Join(os.Getenv("HOME"), "missing"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace path does not exist")
}

func TestSetupWorkspace_NotDirectory(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	filePath := filepath.Join(homeDir, "not-a-directory")
	require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o600))

	err := SetupWorkspace(filePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace path is not a directory")
}

func TestSetupWorkspace_NestedPathsAreStoredSeparately(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	parentDir := filepath.Join(homeDir, "work")
	nestedDir := filepath.Join(parentDir, "client-x")
	require.NoError(t, os.MkdirAll(nestedDir, 0o700))

	require.NoError(t, SetupWorkspace(parentDir))
	require.NoError(t, SetupWorkspace(nestedDir))

	config, err := workspacecfg.LoadConfig(UserConfigPath())
	require.NoError(t, err)
	assert.Equal(t, []string{"~/work", "~/work/client-x"}, config.Workspaces)
}

func TestTeardownWorkspaceWithReport(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	require.NoError(t, SetupWorkspace(workspaceDir))

	result, err := TeardownWorkspaceWithReport(workspaceDir)
	require.NoError(t, err)
	assert.True(t, result.ToreDownGlobalResources)
	assert.Contains(t, result.Report.AffectedPaths, "~/.config/argus/")
	assert.Contains(t, result.Report.AffectedPaths, "~/.config/argus/config.yaml")
	assert.Contains(t, result.Report.Changes.Removed, "~/.config/argus/")
	assert.Contains(t, result.Report.Changes.Removed, "~/.claude/skills/argus-*")
	_, err = os.Stat(filepath.Join(homeDir, ".config", "argus"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestTeardownWorkspaceWithReport_LastWorkspaceFailureKeepsRegistration(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	require.NoError(t, SetupWorkspace(workspaceDir))

	_, err := teardownWorkspaceWithReport(workspaceDir, workspaceTeardownOps{
		teardownArtifacts: func(_ string, _ *mutationTracker) error {
			return errors.New("boom")
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tearing down global artifacts")

	config, loadErr := workspacecfg.LoadConfig(UserConfigPath())
	require.NoError(t, loadErr)
	assert.Equal(t, []string{"~/work"}, config.Workspaces)

	result, err := TeardownWorkspaceWithReport(workspaceDir)
	require.NoError(t, err)
	assert.True(t, result.ToreDownGlobalResources)
	_, err = os.Stat(filepath.Join(homeDir, ".config", "argus"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSetupGlobalHooks_ClaudeCode(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, SetupGlobalHooks([]string{agentClaudeCode}))

	settings := readJSONFile(t, filepath.Join(homeDir, claudeSettingsRelativePath))
	claudeCommands := hookCommandsForEvent(t, settings, "UserPromptSubmit")
	require.Len(t, claudeCommands, 1)
	assertArgusShellHookCommand(t, claudeCommands[0], "claude-code", true)
	assert.Empty(t, hookCommandsForEvent(t, settings, "PreToolUse"))
}

func TestSetupGlobalHooks_Codex(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, SetupGlobalHooks([]string{agentCodex}))

	hooks := readJSONFile(t, filepath.Join(homeDir, codexHooksRelativePath))
	codexCommands := hookCommandsForEvent(t, hooks, "UserPromptSubmit")
	require.Len(t, codexCommands, 1)
	assertArgusShellHookCommand(t, codexCommands[0], "codex", true)
	assert.Empty(t, hookCommandsForEvent(t, hooks, "PreToolUse"))

	config := readTOMLFile(t, filepath.Join(homeDir, codexConfigRelativePath))
	assert.Equal(t, map[string]any{"codex_hooks": true}, requireTOMLMap(t, config["features"]))
}

func TestSetupGlobalHooks_OpenCode(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, SetupGlobalHooks([]string{agentOpenCode}))

	pluginPath := filepath.Join(homeDir, ".config", "opencode", "plugins", "argus.ts")
	//nolint:gosec // Test reads a plugin file created under its temp HOME directory.
	plugin, err := os.ReadFile(pluginPath)
	require.NoError(t, err)
	assert.Contains(t, string(plugin), "argus tick --agent opencode --global")
	assert.Contains(t, string(plugin), "parentID: session.data?.parentID")
	assert.Contains(t, string(plugin), "synthetic: true")
	assert.NotContains(t, string(plugin), "argus trap --agent opencode --global")
}

func TestSetupGlobalSkills(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, SetupGlobalSkills())

	assertGlobalSkillsReleased(t)

	for _, skillPath := range GlobalSkillPaths() {
		_, err := os.Stat(filepath.Join(skillPath, "argus-status", "SKILL.md"))
		assert.True(t, os.IsNotExist(err))
	}

	assert.Equal(t, 21, countSkillMarkdownFiles(t, GlobalSkillPaths()))
}

func TestGlobalSkillNames(t *testing.T) {
	assert.True(t, slices.Equal(
		[]string{"argus-configure-invariant", "argus-configure-workflow", "argus-doctor", "argus-intro", "argus-runtime", "argus-setup", "argus-teardown"},
		GlobalSkillNames(),
	))
}

func TestSetupWorkspace_RefreshesGlobalResources(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work", "company")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	require.NoError(t, SetupWorkspace(workspaceDir))

	for _, skillPath := range GlobalSkillPaths() {
		require.NoError(t, os.MkdirAll(filepath.Join(skillPath, "argus-concepts"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(skillPath, "argus-concepts", "SKILL.md"), []byte("# legacy\n"), 0o600))
	}

	introPath := filepath.Join(homeDir, ".agents", "skills", "argus-intro", "SKILL.md")
	require.NoError(t, os.WriteFile(introPath, []byte("# stale intro\n"), 0o600))

	result, err := SetupWorkspaceWithReport(workspaceDir)
	require.NoError(t, err)
	assert.True(t, result.AlreadyRegistered)
	assert.Contains(t, result.Report.AffectedPaths, "~/.config/argus/{invariants,workflows,pipelines,logs}/")
	assert.NotEmpty(t, result.Report.Changes.Updated)
	assert.NotEmpty(t, result.Report.Changes.Removed)

	for _, skillPath := range GlobalSkillPaths() {
		_, statErr := os.Stat(filepath.Join(skillPath, "argus-concepts"))
		assert.True(t, os.IsNotExist(statErr), "%s/argus-concepts should be pruned", skillPath)
		_, statErr = os.Stat(filepath.Join(skillPath, "argus-intro", "SKILL.md"))
		require.NoError(t, statErr, "%s/argus-intro/SKILL.md should exist", skillPath)
	}
}

func TestSetupWorkspace_PrunesObsoleteManagedYAML(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workspaceDir := filepath.Join(homeDir, "work", "company")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	require.NoError(t, SetupWorkspace(workspaceDir))

	globalRoot := filepath.Join(homeDir, ".config", "argus")
	writeTestFile(t, filepath.Join(globalRoot, "invariants", "argus-init.yaml"), "version: v0.1.0\nid: argus-init\norder: 10\nprompt: legacy\ncheck:\n  - shell: true\n")
	writeTestFile(t, filepath.Join(globalRoot, "invariants", "argus-project-init.yaml"), "version: v0.1.0\nid: argus-project-init\norder: 20\nprompt: legacy\ncheck:\n  - shell: true\n")
	writeTestFile(t, filepath.Join(globalRoot, "workflows", "argus-project-init.yaml"), "version: v0.1.0\nid: argus-project-init\njobs:\n  - id: legacy\n    prompt: legacy\n")
	writeTestFile(t, filepath.Join(globalRoot, "invariants", "team-check.yaml"), "version: v0.1.0\nid: team-check\norder: 30\nprompt: keep\ncheck:\n  - shell: true\n")
	writeTestFile(t, filepath.Join(globalRoot, "workflows", "team-workflow.yaml"), "version: v0.1.0\nid: team-workflow\njobs:\n  - id: keep\n    prompt: keep\n")

	_, err := SetupWorkspaceWithReport(workspaceDir)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(globalRoot, "invariants", "argus-init.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(globalRoot, "invariants", "argus-project-init.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(globalRoot, "workflows", "argus-project-init.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, err = os.Stat(filepath.Join(globalRoot, "invariants", "argus-project-setup.yaml"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(globalRoot, "invariants", "team-check.yaml"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(globalRoot, "workflows", "team-workflow.yaml"))
	require.NoError(t, err)
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
			//nolint:gosec // Test reads a released skill file from its temp HOME directory.
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
