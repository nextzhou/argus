package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckInstallPreconditionsRequiresGitRepository(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	nonGitDir := t.TempDir()
	t.Chdir(nonGitDir)

	projectRoot, isSubdir, err := CheckInstallPreconditions()
	require.Error(t, err)
	assert.Empty(t, projectRoot)
	assert.False(t, isSubdir)
	assert.Contains(t, err.Error(), "git repository")
}

func TestCheckInstallPreconditionsRejectsNestedInstall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := newTestProjectRoot(t)
	require.NoError(t, os.Mkdir(filepath.Join(projectRoot, ".argus"), 0o755))

	nestedDir := filepath.Join(projectRoot, "services", "api")
	require.NoError(t, os.MkdirAll(nestedDir, 0o755))
	t.Chdir(nestedDir)

	_, _, err := CheckInstallPreconditions()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ancestor .argus/")
	assert.Contains(t, err.Error(), projectRoot)
}

func TestCheckInstallPreconditionsSubdirectoryDetection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := newTestProjectRoot(t)
	subdir := filepath.Join(projectRoot, "pkg", "cli")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	t.Run("git root", func(t *testing.T) {
		t.Chdir(projectRoot)

		gotRoot, isSubdir, err := CheckInstallPreconditions()
		require.NoError(t, err)
		assert.Equal(t, projectRoot, gotRoot)
		assert.False(t, isSubdir)
	})

	t.Run("git subdirectory", func(t *testing.T) {
		t.Chdir(subdir)

		gotRoot, isSubdir, err := CheckInstallPreconditions()
		require.NoError(t, err)
		assert.Equal(t, subdir, gotRoot)
		assert.True(t, isSubdir)
	})
}

func TestCheckInstallPreconditionsAcceptsGitFileMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".git"), []byte("gitdir: /tmp/example\n"), 0o644))
	t.Chdir(projectRoot)

	gotRoot, isSubdir, err := CheckInstallPreconditions()
	require.NoError(t, err)
	assert.Equal(t, projectRoot, gotRoot)
	assert.False(t, isSubdir)
}

func TestInstallCreatesProjectStructureAndAssets(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	projectRoot := newTestProjectRoot(t)

	result, err := InstallWithReport(projectRoot)
	require.NoError(t, err)
	assert.Equal(t, projectRoot, result.Root)
	assert.Contains(t, result.Report.AffectedPaths, ".argus/{workflows,invariants,rules,pipelines,logs,data,tmp}/")
	assert.Contains(t, result.Report.Changes.Created, ".argus/{workflows,invariants,rules,pipelines,logs,data,tmp}/")

	for _, relDir := range []string{
		".argus/workflows",
		".argus/invariants",
		".argus/rules",
		".argus/pipelines",
		".argus/logs",
		".argus/data",
		".argus/tmp",
	} {
		assert.DirExists(t, filepath.Join(projectRoot, relDir))
	}

	assertReleasedAsset(t, projectRoot, "workflows/argus-init.yaml", ".argus/workflows/argus-init.yaml")
	assertReleasedAsset(t, projectRoot, "invariants/argus-init.yaml", ".argus/invariants/argus-init.yaml")
	for _, skillPath := range SkillPaths() {
		assertReleasedAsset(t, projectRoot, "skills/argus-doctor/SKILL.md", filepath.Join(skillPath, "argus-doctor", "SKILL.md"))
	}

	settings := readJSONFile(t, filepath.Join(projectRoot, ".claude", "settings.json"))
	assert.Equal(t, []string{"argus tick --agent claude-code"}, hookCommandsForEvent(t, settings, "UserPromptSubmit"))
	assert.Empty(t, hookCommandsForEvent(t, settings, "PreToolUse"))

	assert.FileExists(t, filepath.Join(projectRoot, ".codex", "hooks.json"))
	assert.FileExists(t, filepath.Join(projectRoot, ".opencode", "plugins", "argus.ts"))
	assert.FileExists(t, filepath.Join(homeDir, ".codex", "config.toml"))
}

func TestInstallIsIdempotent(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	projectRoot := newTestProjectRoot(t)

	_, err := InstallWithReport(projectRoot)
	require.NoError(t, err)

	result, err := InstallWithReport(projectRoot)
	require.NoError(t, err)
	assert.Empty(t, result.Report.Changes.Created)
	assert.Empty(t, result.Report.Changes.Updated)
	assert.Empty(t, result.Report.Changes.Removed)
	assert.Contains(t, result.Report.AffectedPaths, ".argus/{workflows,invariants,rules,pipelines,logs,data,tmp}/")

	settings := readJSONFile(t, filepath.Join(projectRoot, ".claude", "settings.json"))
	assert.Equal(t, []string{"argus tick --agent claude-code"}, hookCommandsForEvent(t, settings, "UserPromptSubmit"))
	assert.Empty(t, hookCommandsForEvent(t, settings, "PreToolUse"))

	for _, skillPath := range SkillPaths() {
		assertReleasedAsset(t, projectRoot, "skills/argus-install/SKILL.md", filepath.Join(skillPath, "argus-install", "SKILL.md"))
	}
	assert.FileExists(t, filepath.Join(homeDir, ".codex", "config.toml"))
}

func TestInstallPrunesObsoleteBuiltinSkills(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	projectRoot := newTestProjectRoot(t)

	for _, skillPath := range SkillPaths() {
		legacySkillDir := filepath.Join(projectRoot, skillPath, "argus-concepts")
		require.NoError(t, os.MkdirAll(legacySkillDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(legacySkillDir, "SKILL.md"), []byte("# legacy\n"), 0o644))
	}

	result, err := InstallWithReport(projectRoot)
	require.NoError(t, err)

	for _, skillPath := range SkillPaths() {
		_, statErr := os.Stat(filepath.Join(projectRoot, skillPath, "argus-concepts"))
		assert.True(t, os.IsNotExist(statErr), "%s/argus-concepts should be pruned", skillPath)
		_, statErr = os.Stat(filepath.Join(projectRoot, skillPath, "argus-intro", "SKILL.md"))
		assert.NoError(t, statErr, "%s/argus-intro/SKILL.md should exist", skillPath)
	}

	assert.Contains(t, result.Report.Changes.Removed, ".agents/skills/argus-*/SKILL.md")
}

func assertReleasedAsset(t *testing.T, projectRoot, srcPath, dstPath string) {
	t.Helper()

	want, err := assets.ReadAsset(srcPath)
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(projectRoot, dstPath))
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got))
}
