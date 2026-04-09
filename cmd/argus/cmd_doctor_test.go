package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/doctor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeDoctorCmd runs the doctor command and captures command output.
func executeDoctorCmd(t *testing.T) (string, error) {
	t.Helper()

	cmd := newDoctorCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmdErr := cmd.Execute()

	return out.String(), cmdErr
}

func TestWriteDoctorReport_TextOutput(t *testing.T) {
	results := []doctor.CheckResult{
		{Name: "install-integrity", Status: "pass"},
		{Name: "workflow-files", Status: "fail", Message: "workflow directory missing", Suggestion: "re-run `argus install`"},
		{Name: "workspace-config", Status: "skip", Message: "no workspace config found"},
	}

	var out bytes.Buffer
	failed := writeDoctorReport(&out, results)
	output := out.String()

	assert.Equal(t, 1, failed)

	assert.Contains(t, output, "[PASS]", "output should contain [PASS] markers")
	assert.Contains(t, output, "[FAIL]", "output should contain [FAIL] markers")
	assert.Contains(t, output, "[SKIP]", "output should contain [SKIP] markers")
	assert.Contains(t, output, "  → re-run `argus install`", "failed checks should include suggestions")

	assert.Regexp(t, `\d+ checks: \d+ passed, \d+ failed, \d+ skipped`, output,
		"output should contain summary line with check counts")
}

func TestDoctorCmd_ExitCodeOnFailure(t *testing.T) {
	setUpIsolatedDoctorEnv(t)

	out, err := executeDoctorCmd(t)

	assert.Error(t, err, "doctor should return error when checks fail")
	assert.Contains(t, err.Error(), "doctor found")
	assert.Contains(t, out, "[FAIL] install-integrity")
	assert.Regexp(t, `\d+ checks: \d+ passed, \d+ failed, \d+ skipped`, out,
		"output should contain summary line with check counts")
}

func TestDoctorCmd_NoProjectRoot(t *testing.T) {
	setUpIsolatedDoctorEnv(t)

	out, _ := executeDoctorCmd(t)

	assert.Contains(t, out, "[SKIP] hook-config: project root not found",
		"should skip project-specific checks when no project root")
	assert.Contains(t, out, "[SKIP] workflow-files: project root not found",
		"should skip project-specific checks when no project root")
	assert.Contains(t, out, "[PASS] tmp-permissions",
		"non-project checks should still run")
	assert.Regexp(t, `\d+ checks: \d+ passed, \d+ failed, \d+ skipped`, out,
		"should still show summary")
}

func TestWriteDoctorReport_OutputFormat(t *testing.T) {
	results := []doctor.CheckResult{
		{Name: "install-integrity", Status: "pass"},
		{Name: "workflow-files", Status: "fail", Message: "workflow directory missing"},
		{Name: "workspace-config", Status: "skip", Message: "no workspace config found"},
	}

	var out bytes.Buffer
	writeDoctorReport(&out, results)
	output := out.String()

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

func setUpIsolatedDoctorEnv(t *testing.T) {
	t.Helper()

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("HOME", filepath.Join(tmpDir, "home"))
	t.Setenv("SHELL", "/bin/bash")
}
