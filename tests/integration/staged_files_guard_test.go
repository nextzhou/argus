package integration

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckStagedFilesScriptFailsForHardcodedSessionIDInCmdTests(t *testing.T) {
	projectDir := initGitRepo(t)
	writeFile(t, projectDir, "cmd/argus/cmd_guard_test.go", stagedSessionJSONFixture("fixed-session"))
	stageAll(t, projectDir)

	output, err := runCheckStagedFilesScript(t, projectDir)
	require.Error(t, err, "script should fail when cmd tests hardcode session literals")
	assert.Contains(t, output, "cmd/argus/cmd_guard_test.go")
	assert.Contains(t, output, "high-risk test hardcodes a session literal")
	assert.Contains(t, output, "sessiontest.NewSessionID")
}

func TestCheckStagedFilesScriptFailsForHardcodedSessionFlagInIntegrationTests(t *testing.T) {
	projectDir := initGitRepo(t)
	writeFile(t, projectDir, "tests/integration/error_guard_test.go", stagedSessionFlagFixture("fixed-session"))
	stageAll(t, projectDir)

	output, err := runCheckStagedFilesScript(t, projectDir)
	require.Error(t, err, "script should fail when integration tests hardcode --session literals")
	assert.Contains(t, output, "tests/integration/error_guard_test.go")
	assert.Contains(t, output, "high-risk test hardcodes a session literal")
	assert.Contains(t, output, "newDefaultSessionID")
}

func TestCheckStagedFilesScriptFailsForDirectTmpArgusLiteral(t *testing.T) {
	projectDir := initGitRepo(t)
	writeFile(t, projectDir, "tests/integration/tmp_guard_test.go", stagedTmpArgusLiteralFixture())
	stageAll(t, projectDir)

	output, err := runCheckStagedFilesScript(t, projectDir)
	require.Error(t, err, "script should fail when tests write "+"/tmp"+"/argus directly")
	assert.Contains(t, output, "tests/integration/tmp_guard_test.go")
	assert.Contains(t, output, tmpArgusGuardFailurePrefix())
	assert.Contains(t, output, "shared cleanup helpers")
}

func TestCheckStagedFilesScriptAllowsTmpArgusLiteralInAllowlistedTests(t *testing.T) {
	tests := []string{
		"tests/integration/helpers_test.go",
		"internal/doctor/checks_test.go",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			projectDir := initGitRepo(t)
			writeFile(t, projectDir, path, stagedTmpArgusLiteralFixture())
			stageAll(t, projectDir)

			output, err := runCheckStagedFilesScript(t, projectDir)
			require.NoError(t, err, "script output: %s", output)
		})
	}
}

func TestCheckStagedFilesScriptAllowsHardcodedSessionLiteralOutsideHighRiskTests(t *testing.T) {
	projectDir := initGitRepo(t)
	writeFile(t, projectDir, "internal/hook/input_test.go", stagedSessionJSONFixture("fixed-session"))
	stageAll(t, projectDir)

	output, err := runCheckStagedFilesScript(t, projectDir)
	require.NoError(t, err, "script output: %s", output)
}

func runCheckStagedFilesScript(t *testing.T, dir string) (string, error) {
	t.Helper()

	cmd := exec.Command("bash", filepath.Join(findProjectRoot(), "scripts", "check-staged-files.sh"))
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func stagedSessionJSONFixture(sessionID string) string {
	return strings.Join([]string{
		"package sample",
		"",
		"func sample() {",
		"\t_ = `" + `{"` + "session_id" + `":"` + sessionID + `"}` + "`",
		"}",
		"",
	}, "\n")
}

func stagedSessionFlagFixture(sessionID string) string {
	return strings.Join([]string{
		"package sample",
		"",
		"func sample() {",
		"\t_ = []string{\"workflow\", \"snooze\", \"" + "--session" + "\", \"" + sessionID + "\"}",
		"}",
		"",
	}, "\n")
}

// Keep the default tmp-session path literal split so this guard self-test does
// not trigger the repository's own staged-file guard before it can stage the
// synthetic file.
func stagedTmpArgusLiteralFixture() string {
	return strings.Join([]string{
		"package sample",
		"",
		"func sample() {",
		"\t_ = \"" + "/tmp" + "/argus\"",
		"}",
		"",
	}, "\n")
}

// Mirror the guard failure prefix without embedding the raw tmp-session path
// literal directly in this file, or the test file itself would trip the guard.
func tmpArgusGuardFailurePrefix() string {
	return "test file writes " + "/tmp" + "/argus directly"
}
