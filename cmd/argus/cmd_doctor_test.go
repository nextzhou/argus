package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeDoctorCmd runs the doctor command and captures stdout output.
// Tests using this helper must NOT call t.Parallel since os.Stdout is redirected.
func executeDoctorCmd(t *testing.T) ([]byte, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := newDoctorCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmdErr := cmd.Execute()

	require.NoError(t, w.Close())
	os.Stdout = old

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return out, cmdErr
}

func TestDoctorCmd_TextOutput(t *testing.T) {
	// Setup: Create minimal .argus directory to simulate a project
	argusDir := ".argus"
	require.NoError(t, os.MkdirAll(argusDir, 0o755))
	t.Cleanup(func() {
		_ = os.RemoveAll(argusDir)
	})

	out, err := executeDoctorCmd(t)
	output := string(out)

	// Verify output contains status markers
	assert.Contains(t, output, "[PASS]", "output should contain [PASS] markers")
	assert.Contains(t, output, "[FAIL]", "output should contain [FAIL] markers")
	assert.Contains(t, output, "[SKIP]", "output should contain [SKIP] markers")

	// Verify summary line exists
	assert.Regexp(t, `\d+ checks: \d+ passed, \d+ failed, \d+ skipped`, output,
		"output should contain summary line with check counts")

	// Verify no error returned (exit code 0 when there are failures is acceptable for doctor)
	// Doctor should return error only if there are failures
	if strings.Contains(output, "[FAIL]") {
		assert.Error(t, err, "should return error when there are failures")
	}
}

func TestDoctorCmd_ExitCodeOnFailure(t *testing.T) {
	// Setup: Create .argus directory but with missing required files to trigger failures
	argusDir := ".argus"
	require.NoError(t, os.MkdirAll(argusDir, 0o755))
	t.Cleanup(func() {
		_ = os.RemoveAll(argusDir)
	})

	out, err := executeDoctorCmd(t)
	output := string(out)

	// If there are failures in output, command should return error
	if strings.Contains(output, "[FAIL]") {
		assert.Error(t, err, "doctor should return error when checks fail")
	}
}

func TestDoctorCmd_NoProjectRoot(t *testing.T) {
	// Change to a directory without .argus or .git
	tmpDir := t.TempDir()
	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
	})

	out, _ := executeDoctorCmd(t)
	output := string(out)

	// Should still run checks, but skip project-specific ones
	assert.Contains(t, output, "[SKIP]", "should have skipped checks when no project root")
	assert.Regexp(t, `\d+ checks: \d+ passed, \d+ failed, \d+ skipped`, output,
		"should still show summary")
}

func TestDoctorCmd_OutputFormat(t *testing.T) {
	// Setup: Create minimal .argus directory
	argusDir := ".argus"
	require.NoError(t, os.MkdirAll(argusDir, 0o755))
	t.Cleanup(func() {
		_ = os.RemoveAll(argusDir)
	})

	out, _ := executeDoctorCmd(t)
	output := string(out)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Greater(t, len(lines), 0, "output should have at least one line")

	// Verify each check line has proper format: [STATUS] Name or [STATUS] Name: message
	for _, line := range lines {
		if strings.HasPrefix(line, "[") {
			// Check line format
			assert.Regexp(t, `^\[(PASS|FAIL|SKIP)\]`, line,
				"check line should start with [PASS], [FAIL], or [SKIP]")
		}
	}

	// Last line should be summary
	lastLine := lines[len(lines)-1]
	assert.Regexp(t, `\d+ checks: \d+ passed, \d+ failed, \d+ skipped`, lastLine,
		"last line should be summary")
}
