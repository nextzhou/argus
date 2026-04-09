package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestError_WorkflowNotFound(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	result = runArgus(t, projectDir, "workflow", "start", "nonexistent")
	data := requireError(t, result)
	assert.Contains(t, data["message"].(string), "nonexistent")
}

func TestError_DuplicateWorkflowStart(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/dup-test.yaml", `version: v0.1.0
id: dup-test
jobs:
  - id: step_one
    prompt: "Do step one"
`)

	result = runArgus(t, projectDir, "workflow", "start", "dup-test")
	requireOK(t, result)

	result = runArgus(t, projectDir, "workflow", "start", "dup-test")
	requireError(t, result)
}

func TestError_JobDoneNoActivePipeline(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	result = runArgus(t, projectDir, "job-done")
	requireError(t, result)
}

func TestError_CancelNoActivePipeline(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	result = runArgus(t, projectDir, "workflow", "cancel")
	requireError(t, result)
}

func TestError_InvalidWorkflowID(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	result = runArgus(t, projectDir, "workflow", "start", "../etc/passwd")
	assert.NotEqual(t, 0, result.ExitCode)
}

func TestError_CorruptYAMLWorkflowInspect(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/corrupt.yaml", `{{{invalid yaml`)

	result = runArgus(t, projectDir, "workflow", "inspect")
	require.Equal(t, 0, result.ExitCode, "inspect should not crash on corrupt YAML")
	data := parseJSON(t, result.Stdout)
	assert.Equal(t, false, data["valid"])
}

func TestError_CorruptYAMLInvariantInspect(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/invariants/corrupt.yaml", `not: [valid: yaml: {{`)

	result = runArgus(t, projectDir, "invariant", "inspect")
	require.Equal(t, 0, result.ExitCode, "inspect should not crash on corrupt YAML")
	data := parseJSON(t, result.Stdout)
	assert.Equal(t, false, data["valid"])
}

func TestError_TickFailOpen(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".argus", "workflows"), 0o755))

	result := runArgusWithStdin(t, projectDir, `{invalid json`, "tick", "--agent", "claude-code")
	assert.Equal(t, 0, result.ExitCode, "tick must always exit 0 (fail-open)")
}

func TestError_TickSubAgentSkip(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/skip-test.yaml", `version: v0.1.0
id: skip-test
jobs:
  - id: step_one
    prompt: "Do it"
`)

	result = runArgus(t, projectDir, "workflow", "start", "skip-test")
	requireOK(t, result)

	stdinJSON := `{"session_id":"sub-agent-test","agent_id":"worker-1"}`
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code")
	require.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Stdout, "sub-agent tick should produce no output")
}

func TestError_DoctorReportsProblems(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".argus", "pipelines"), 0o755))

	writeFile(t, projectDir, ".argus/pipelines/bad-1-20240101T000000Z.yaml",
		`version: v0.1.0
workflow_id: bad-1
status: running
current_job: step1
started_at: "20240101T000000Z"
jobs:
  step1:
    started_at: "20240101T000000Z"
`)
	writeFile(t, projectDir, ".argus/pipelines/bad-2-20240101T000001Z.yaml",
		`version: v0.1.0
workflow_id: bad-2
status: running
current_job: step1
started_at: "20240101T000001Z"
jobs:
  step1:
    started_at: "20240101T000001Z"
`)

	result := runArgusText(t, projectDir, "doctor")
	assert.NotEqual(t, 0, result.ExitCode, "doctor should exit non-zero when issues found")
	assert.Contains(t, result.Stdout, "FAIL")
}

func TestError_UninstallNotInstalled(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "uninstall", "--yes")
	assert.NotEqual(t, 0, result.ExitCode)
}

func TestError_InstallNonGitDirectory(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	nonGitDir := t.TempDir()

	result := runArgus(t, nonGitDir, "install", "--yes")
	assert.NotEqual(t, 0, result.ExitCode)
}

func TestError_SnoozeNoActivePipeline(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	result = runArgus(t, projectDir, "workflow", "snooze", "--session", "test-session")
	requireError(t, result)
}

func TestError_RecoverAfterError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/recover-test.yaml", `version: v0.1.0
id: recover-test
jobs:
  - id: step_one
    prompt: "Do it"
`)

	result = runArgus(t, projectDir, "workflow", "start", "recover-test")
	requireOK(t, result)

	result = runArgus(t, projectDir, "job-done", "--fail", "--message", "oops")
	data := requireOK(t, result)
	assert.Equal(t, "failed", data["pipeline_status"])

	result = runArgus(t, projectDir, "workflow", "start", "recover-test")
	data = requireOK(t, result)
	assert.Equal(t, "running", data["pipeline_status"])
	assert.Equal(t, "1/1", data["progress"])
}

func TestError_ErrorEnvelopeFormat(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	result = runArgus(t, projectDir, "workflow", "start", "does-not-exist")
	assert.NotEqual(t, 0, result.ExitCode)

	data := parseJSON(t, result.Stdout)
	assert.Equal(t, "error", data["status"])
	_, hasMessage := data["message"]
	assert.True(t, hasMessage, "error envelope must contain 'message' field")
}

func TestError_InvariantCheckNotFound(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	result = runArgus(t, projectDir, "invariant", "check", "nonexistent-inv")
	assert.NotEqual(t, 0, result.ExitCode)
}

func TestError_WorkflowMissingFile(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/missing-ref.yaml", `version: v0.1.0
id: missing-ref
jobs:
  - ref: nonexistent_shared_job
`)

	result = runArgus(t, projectDir, "workflow", "start", "missing-ref")
	assert.NotEqual(t, 0, result.ExitCode)
}

func TestError_JobDoneAfterPipelineCompleted(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/done-test.yaml", `version: v0.1.0
id: done-test
jobs:
  - id: only_step
    prompt: "Do the thing"
`)

	result = runArgus(t, projectDir, "workflow", "start", "done-test")
	requireOK(t, result)

	result = runArgus(t, projectDir, "job-done", "--message", "completed")
	data := requireOK(t, result)
	assert.Equal(t, "completed", data["pipeline_status"])

	result = runArgus(t, projectDir, "job-done")
	requireError(t, result)
}

func TestError_TrapAlwaysExitZero(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := t.TempDir()

	result := runArgusWithStdin(t, projectDir, `{}`, "trap", "--agent", "claude-code")
	assert.Equal(t, 0, result.ExitCode)

	result = runArgusWithStdin(t, projectDir, `{}`, "trap", "--agent", "codex")
	assert.Equal(t, 0, result.ExitCode)

	result = runArgusWithStdin(t, projectDir, `{}`, "trap", "--agent", "opencode")
	assert.Equal(t, 0, result.ExitCode)

	result = runArgusWithStdin(t, projectDir, "", "trap", "--agent", "claude-code")
	assert.Equal(t, 0, result.ExitCode)
}

func TestError_FailEndPipelineCombination(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/combo-test.yaml", `version: v0.1.0
id: combo-test
jobs:
  - id: step_one
    prompt: "Do it"
  - id: step_two
    prompt: "Do more"
`)

	result = runArgus(t, projectDir, "workflow", "start", "combo-test")
	requireOK(t, result)

	result = runArgus(t, projectDir, "job-done", "--fail", "--end-pipeline", "--message", "bad ending")
	data := requireOK(t, result)
	assert.Equal(t, "failed", data["pipeline_status"])

	result = runArgus(t, projectDir, "status")
	data = requireOK(t, result)
	assert.Nil(t, data["pipeline"])
}
