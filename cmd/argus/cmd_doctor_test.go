package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/doctor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeDoctorCmd runs the doctor command and captures command output.
func executeDoctorCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := newDoctorCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	cmdErr := cmd.Execute()

	return out.String(), cmdErr
}

func TestWriteDoctorReport_TextOutput(t *testing.T) {
	results := []doctor.CheckResult{
		{Name: "setup-integrity", Status: "pass"},
		{Name: "workflow-files", Status: "fail", Message: "workflow directory missing", Suggestion: "re-run `argus setup`"},
		{Name: "workspace-config", Status: "skip", Message: "no workspace config found"},
	}

	var out bytes.Buffer
	failed := writeDoctorReport(&out, results)
	output := out.String()

	assert.Equal(t, 1, failed)

	assert.Contains(t, output, "[PASS]", "output should contain [PASS] markers")
	assert.Contains(t, output, "[FAIL]", "output should contain [FAIL] markers")
	assert.Contains(t, output, "[SKIP]", "output should contain [SKIP] markers")
	assert.Contains(t, output, "  → re-run `argus setup`", "failed checks should include suggestions")

	assert.Regexp(t, `\d+ checks: \d+ passed, \d+ failed, \d+ skipped`, output,
		"output should contain summary line with check counts")
}

func TestDoctorCmd_ExitCodeOnFailure(t *testing.T) {
	setUpIsolatedDoctorEnv(t)

	out, err := executeDoctorCmd(t)

	require.Error(t, err, "doctor should return error when checks fail")
	assert.Contains(t, err.Error(), "doctor found")
	assert.Contains(t, out, "[FAIL] setup-integrity")
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

func TestDoctorCmd_DefaultShowsInvariantDiagnosticsSkip(t *testing.T) {
	setUpIsolatedDoctorEnv(t)
	projectRoot := t.TempDir()
	t.Chdir(projectRoot)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o700))

	out, _ := executeDoctorCmd(t)

	assert.Contains(t, out, "[SKIP] automatic-invariant-diagnostics")
	assert.Contains(t, out, "disabled by default")
	assert.Contains(t, out, "argus doctor --check-invariants")
}

func TestDoctorCmd_JSONIncludesStructuredInvariantDiagnostics(t *testing.T) {
	setUpIsolatedDoctorEnv(t)
	projectRoot := t.TempDir()
	t.Chdir(projectRoot)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".argus", "invariants"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".argus", "invariants", "slow-check.yaml"), []byte(`version: v0.1.0
id: slow-check
order: 10
auto: always
check:
  - shell: "sleep 3"
prompt: "Fix it"
`), 0o600))

	data, err := executeJSONCommand(t, newDoctorCmd(), "--check-invariants")
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	checks := payload["checks"].([]any)
	found := false
	for _, raw := range checks {
		check := raw.(map[string]any)
		if check["name"] != "automatic-invariant-diagnostics" {
			continue
		}
		found = true
		detail := check["detail"].(map[string]any)
		auto := detail["automatic_invariant_diagnostics"].(map[string]any)
		assert.Equal(t, true, auto["enabled"])
		invariants := auto["invariants"].([]any)
		require.Len(t, invariants, 1)
		break
	}
	assert.True(t, found, "expected automatic invariant diagnostics check in JSON output")
}

func TestWriteDoctorReport_OutputFormat(t *testing.T) {
	results := []doctor.CheckResult{
		{Name: "setup-integrity", Status: "pass"},
		{Name: "workflow-files", Status: "fail", Message: "workflow directory missing"},
		{Name: "workspace-config", Status: "skip", Message: "no workspace config found"},
	}

	var out bytes.Buffer
	writeDoctorReport(&out, results)
	output := out.String()

	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.NotEmpty(t, lines, "output should have at least one line")

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
