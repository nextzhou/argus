package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindProjectRoot_ArgusAtCWD(t *testing.T) {
	base := t.TempDir()
	argusDir := filepath.Join(base, ".argus")
	require.NoError(t, os.Mkdir(argusDir, 0o700))

	root, err := FindProjectRoot(base)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.True(t, root.HasArgus)
	assert.False(t, root.HasGit)
}

func TestFindProjectRoot_ArgusInParent(t *testing.T) {
	base := t.TempDir()
	argusDir := filepath.Join(base, ".argus")
	require.NoError(t, os.Mkdir(argusDir, 0o700))

	subdir := filepath.Join(base, "subdir", "nested")
	require.NoError(t, os.MkdirAll(subdir, 0o700))

	root, err := FindProjectRoot(subdir)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.True(t, root.HasArgus)
	assert.False(t, root.HasGit)
}

func TestFindProjectRoot_GitAtCWD(t *testing.T) {
	base := t.TempDir()
	gitDir := filepath.Join(base, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o700))

	root, err := FindProjectRoot(base)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.False(t, root.HasArgus)
	assert.True(t, root.HasGit)
}

func TestFindProjectRoot_GitFileAtCWD(t *testing.T) {
	base := t.TempDir()
	gitFile := filepath.Join(base, ".git")
	require.NoError(t, os.WriteFile(gitFile, []byte("gitdir: /tmp/worktrees/example\n"), 0o600))

	root, err := FindProjectRoot(base)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.False(t, root.HasArgus)
	assert.True(t, root.HasGit)
}

func TestFindProjectRoot_GitInParent(t *testing.T) {
	base := t.TempDir()
	gitDir := filepath.Join(base, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o700))

	subdir := filepath.Join(base, "subdir", "nested")
	require.NoError(t, os.MkdirAll(subdir, 0o700))

	root, err := FindProjectRoot(subdir)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.False(t, root.HasArgus)
	assert.True(t, root.HasGit)
}

func TestFindProjectRoot_GitFileInParent(t *testing.T) {
	base := t.TempDir()
	gitFile := filepath.Join(base, ".git")
	require.NoError(t, os.WriteFile(gitFile, []byte("gitdir: /tmp/worktrees/example\n"), 0o600))

	subdir := filepath.Join(base, "subdir", "nested")
	require.NoError(t, os.MkdirAll(subdir, 0o700))

	root, err := FindProjectRoot(subdir)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.False(t, root.HasArgus)
	assert.True(t, root.HasGit)
}

func TestFindProjectRoot_BothArgusAndGit(t *testing.T) {
	base := t.TempDir()
	argusDir := filepath.Join(base, ".argus")
	gitDir := filepath.Join(base, ".git")
	require.NoError(t, os.Mkdir(argusDir, 0o700))
	require.NoError(t, os.Mkdir(gitDir, 0o700))

	root, err := FindProjectRoot(base)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.True(t, root.HasArgus)
	assert.True(t, root.HasGit)
}

func TestFindProjectRoot_BothArgusAndGitFile(t *testing.T) {
	base := t.TempDir()
	argusDir := filepath.Join(base, ".argus")
	gitFile := filepath.Join(base, ".git")
	require.NoError(t, os.Mkdir(argusDir, 0o700))
	require.NoError(t, os.WriteFile(gitFile, []byte("gitdir: /tmp/worktrees/example\n"), 0o600))

	root, err := FindProjectRoot(base)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.True(t, root.HasArgus)
	assert.True(t, root.HasGit)
}

func TestFindProjectRoot_NeitherFound(t *testing.T) {
	base := t.TempDir()
	subdir := filepath.Join(base, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0o700))

	root, err := FindProjectRoot(subdir)
	require.NoError(t, err)
	assert.Nil(t, root)
}

func TestFindProjectRoot_ArgusWinsOverGit(t *testing.T) {
	base := t.TempDir()
	argusDir := filepath.Join(base, ".argus")
	gitDir := filepath.Join(base, ".git")
	require.NoError(t, os.Mkdir(argusDir, 0o700))
	require.NoError(t, os.Mkdir(gitDir, 0o700))

	subdir := filepath.Join(base, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0o700))

	// Create .git in subdir but not .argus
	subGitDir := filepath.Join(subdir, ".git")
	require.NoError(t, os.Mkdir(subGitDir, 0o700))

	// Should find parent .argus, not subdir .git
	root, err := FindProjectRoot(subdir)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.True(t, root.HasArgus)
}

func TestFindProjectRoot_NestedArgus(t *testing.T) {
	base := t.TempDir()
	parentArgus := filepath.Join(base, ".argus")
	require.NoError(t, os.Mkdir(parentArgus, 0o700))

	subdir := filepath.Join(base, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0o700))

	childArgus := filepath.Join(subdir, ".argus")
	require.NoError(t, os.Mkdir(childArgus, 0o700))

	// From subdir, should find child .argus (closest match)
	root, err := FindProjectRoot(subdir)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, subdir, root.Path)
	assert.True(t, root.HasArgus)
}

func TestFindProjectRoot_RelativePath(t *testing.T) {
	base := t.TempDir()
	argusDir := filepath.Join(base, ".argus")
	require.NoError(t, os.Mkdir(argusDir, 0o700))

	t.Chdir(base)

	// Use relative path
	root, err := FindProjectRoot(".")
	require.NoError(t, err)
	require.NotNil(t, root)
	// Resolve symlinks for comparison (handles macOS /var vs /private/var)
	expectedAbs, _ := filepath.EvalSymlinks(base)
	actualAbs, _ := filepath.EvalSymlinks(root.Path)
	assert.Equal(t, expectedAbs, actualAbs)
	assert.True(t, root.HasArgus)
}

func TestFindProjectRoot_DeepNesting(t *testing.T) {
	base := t.TempDir()
	argusDir := filepath.Join(base, ".argus")
	require.NoError(t, os.Mkdir(argusDir, 0o700))

	// Create deeply nested subdirectory
	deepPath := filepath.Join(base, "a", "b", "c", "d", "e")
	require.NoError(t, os.MkdirAll(deepPath, 0o700))

	root, err := FindProjectRoot(deepPath)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.True(t, root.HasArgus)
}

func TestFindProjectRoot_ArgusInParentGitInChild(t *testing.T) {
	base := t.TempDir()
	argusDir := filepath.Join(base, ".argus")
	require.NoError(t, os.Mkdir(argusDir, 0o700))

	subdir := filepath.Join(base, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0o700))

	gitDir := filepath.Join(subdir, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o700))

	// From subdir, should find parent .argus (priority over local .git)
	root, err := FindProjectRoot(subdir)
	require.NoError(t, err)
	require.NotNil(t, root)
	assert.Equal(t, base, root.Path)
	assert.True(t, root.HasArgus)
	assert.False(t, root.HasGit)
}

func TestIsSubdirectory_ExactMatch(t *testing.T) {
	base := t.TempDir()
	assert.True(t, IsSubdirectory(base, base))
}

func TestIsSubdirectory_DirectChild(t *testing.T) {
	base := t.TempDir()
	child := filepath.Join(base, "child")
	require.NoError(t, os.Mkdir(child, 0o700))

	assert.True(t, IsSubdirectory(base, child))
}

func TestIsSubdirectory_DeepChild(t *testing.T) {
	base := t.TempDir()
	deep := filepath.Join(base, "a", "b", "c")
	require.NoError(t, os.MkdirAll(deep, 0o700))

	assert.True(t, IsSubdirectory(base, deep))
}

func TestIsSubdirectory_NotChild(t *testing.T) {
	base := t.TempDir()
	other := t.TempDir()

	assert.False(t, IsSubdirectory(base, other))
}

func TestIsSubdirectory_PrefixButNotSegment(t *testing.T) {
	base := t.TempDir()
	// Create a sibling with similar prefix
	parent := filepath.Dir(base)
	baseName := filepath.Base(base)
	similar := filepath.Join(parent, baseName+"_extra")
	require.NoError(t, os.Mkdir(similar, 0o700))

	assert.False(t, IsSubdirectory(base, similar))
}

func TestIsSubdirectory_RelativePaths(t *testing.T) {
	base := t.TempDir()
	t.Chdir(base)

	child := filepath.Join(base, "child")
	require.NoError(t, os.Mkdir(child, 0o700))

	// Use relative paths
	assert.True(t, IsSubdirectory(".", "child"))
}

func TestIsSubdirectory_InvalidPath(t *testing.T) {
	// Non-existent paths should still work (we don't check existence)
	assert.True(t, IsSubdirectory("/tmp/parent", "/tmp/parent/child"))
	assert.False(t, IsSubdirectory("/tmp/parent", "/tmp/other"))
}
