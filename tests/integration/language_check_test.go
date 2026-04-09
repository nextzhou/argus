package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLanguageCheckScriptPassesForEnglishOnlyFiles(t *testing.T) {
	projectDir := initGitRepo(t)
	writeFile(t, projectDir, "README.md", "# Argus\n\nEnglish only.\n")

	stageAll(t, projectDir)

	cmd := exec.Command("bash", filepath.Join(findProjectRoot(), "scripts", "check-english-only.sh"))
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "script output: %s", output)
}

func TestLanguageCheckScriptFailsForHanCharacters(t *testing.T) {
	projectDir := initGitRepo(t)
	writeFile(t, projectDir, "README.md", "# Argus\n\n\u4e2d\u6587\n")

	stageAll(t, projectDir)

	cmd := exec.Command("bash", filepath.Join(findProjectRoot(), "scripts", "check-english-only.sh"))
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	require.Error(t, err, "script should fail when Han characters are present")
	assert.Contains(t, string(output), "README.md")
	assert.Contains(t, string(output), "Han characters")
}

func initGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init failed: %s", output)

	return dir
}

func stageAll(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git add failed: %s", output)
}
