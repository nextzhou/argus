package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWorkflow = `version: v0.1.0
id: e2e-test
description: End-to-end test workflow
jobs:
  - id: step_one
    prompt: "Execute step one"
  - id: step_two
    prompt: "Execute step two using result from {{.jobs.step_one.message}}"
  - id: step_three
    prompt: "Execute final step"
`

const testInvariant = `version: v0.1.0
id: e2e-test-inv
description: End-to-end test invariant
auto: always
check:
  - shell: "test -f .argus/data/marker.txt"
    description: "marker file exists"
prompt: "Create the marker file at .argus/data/marker.txt"
`

func assertHookSafeTickText(t *testing.T, output string) {
	t.Helper()

	trimmed := strings.TrimLeft(output, " \t\r\n")
	require.NotEmpty(t, trimmed)
	assert.NotEqual(t, '[', rune(trimmed[0]))
	assert.NotEqual(t, '{', rune(trimmed[0]))
}

func TestE2E_CompleteWorkflowLifecycle(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)
	sessionID := newDefaultSessionID(t, "e2e-complete")

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)
	require.True(t, fileExists(t, filepath.Join(projectDir, ".argus", "workflows")))
	require.True(t, fileExists(t, filepath.Join(projectDir, ".argus", "invariants")))
	require.True(t, fileExists(t, filepath.Join(projectDir, ".argus", "pipelines")))

	writeFile(t, projectDir, ".argus/workflows/e2e-test.yaml", testWorkflow)

	result = runArgus(t, projectDir, "workflow", "list")
	data := requireOK(t, result)
	workflows, ok := data["workflows"].([]any)
	require.True(t, ok, "workflows should be an array")
	found := false
	for _, w := range workflows {
		wf := w.(map[string]any)
		if wf["id"] == "e2e-test" {
			found = true
			break
		}
	}
	assert.True(t, found, "e2e-test workflow should appear in list")

	result = runArgus(t, projectDir, "workflow", "start", "e2e-test")
	data = requireOK(t, result)
	assert.Equal(t, "running", data["pipeline_status"])
	assert.Equal(t, "1/3", data["progress"])
	nextJob := data["next_job"].(map[string]any)
	assert.Equal(t, "step_one", nextJob["id"])
	assert.Contains(t, nextJob["prompt"].(string), "Execute step one")

	stdinJSON := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code")
	require.Equal(t, 0, result.ExitCode)
	assertHookSafeTickText(t, result.Stdout)
	assert.Contains(t, result.Stdout, "Argus:")
	assert.Contains(t, result.Stdout, "step_one")
	assert.Contains(t, result.Stdout, "argus job-done")

	result = runArgus(t, projectDir, "job-done", "--message", "step one completed")
	data = requireOK(t, result)
	assert.Equal(t, "running", data["pipeline_status"])
	nextJob = data["next_job"].(map[string]any)
	assert.Equal(t, "step_two", nextJob["id"])
	assert.Contains(t, nextJob["prompt"].(string), "step one completed")

	result = runArgus(t, projectDir, "job-done", "--message", "step two completed")
	data = requireOK(t, result)
	assert.Equal(t, "running", data["pipeline_status"])
	nextJob = data["next_job"].(map[string]any)
	assert.Equal(t, "step_three", nextJob["id"])

	result = runArgus(t, projectDir, "status")
	data = requireOK(t, result)
	pipeline := data["pipeline"].(map[string]any)
	assert.Equal(t, "running", pipeline["status"])
	assert.Equal(t, "e2e-test", pipeline["workflow_id"])
	progress := pipeline["progress"].(map[string]any)
	assert.Equal(t, float64(3), progress["current"])
	assert.Equal(t, float64(3), progress["total"])

	result = runArgus(t, projectDir, "job-done", "--message", "all done")
	data = requireOK(t, result)
	assert.Equal(t, "completed", data["pipeline_status"])
	assert.Equal(t, "3/3", data["progress"])
	assert.Nil(t, data["next_job"])

	result = runArgus(t, projectDir, "status")
	data = requireOK(t, result)
	assert.Nil(t, data["pipeline"])

	result = runArgus(t, projectDir, "uninstall", "--yes")
	requireOK(t, result)
	require.False(t, fileExists(t, filepath.Join(projectDir, ".argus")))
}

func TestE2E_InvariantCheckIntegration(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/invariants/e2e-test-inv.yaml", testInvariant)

	result = runArgus(t, projectDir, "invariant", "list")
	data := requireOK(t, result)
	invariants := data["invariants"].([]any)
	found := false
	for _, inv := range invariants {
		invMap := inv.(map[string]any)
		if invMap["id"] == "e2e-test-inv" {
			found = true
			break
		}
	}
	assert.True(t, found, "e2e-test-inv should appear in list")

	result = runArgus(t, projectDir, "invariant", "check", "e2e-test-inv")
	data = requireOK(t, result)
	assert.Equal(t, float64(0), data["passed"])
	assert.Equal(t, float64(1), data["failed"])
	results := data["results"].([]any)
	require.Len(t, results, 1)
	invResult := results[0].(map[string]any)
	assert.Equal(t, "e2e-test-inv", invResult["id"])
	assert.Equal(t, "failed", invResult["status"])

	writeFile(t, projectDir, ".argus/data/marker.txt", "present")

	result = runArgus(t, projectDir, "invariant", "check", "e2e-test-inv")
	data = requireOK(t, result)
	assert.Equal(t, float64(1), data["passed"])
	assert.Equal(t, float64(0), data["failed"])
	results = data["results"].([]any)
	require.Len(t, results, 1)
	invResult = results[0].(map[string]any)
	assert.Equal(t, "passed", invResult["status"])
}

func TestE2E_WorkflowInspect(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/e2e-test.yaml", testWorkflow)

	result = runArgus(t, projectDir, "workflow", "inspect")
	data := requireOK(t, result)
	files := data["files"].(map[string]any)
	e2eFile := files["e2e-test.yaml"].(map[string]any)
	assert.Equal(t, true, e2eFile["valid"], "user workflow should be valid")

	result = runArgus(t, projectDir, "invariant", "inspect")
	requireOK(t, result)
}

func TestE2E_TickMultiAgentFormats(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)
	claudeSessionID := newDefaultSessionID(t, "agent-claude")
	codexSessionID := newDefaultSessionID(t, "agent-codex")
	opencodeSessionID := newDefaultSessionID(t, "agent-opencode")

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/e2e-test.yaml", testWorkflow)

	result = runArgus(t, projectDir, "workflow", "start", "e2e-test")
	requireOK(t, result)

	claudeStdin := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, claudeSessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, claudeStdin, "tick", "--agent", "claude-code")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "step_one")

	codexStdin := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, codexSessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, codexStdin, "tick", "--agent", "codex")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "step_one")

	opencodeStdin := fmt.Sprintf(`{"sessionID":"%s","cwd":"%s"}`, opencodeSessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, opencodeStdin, "tick", "--agent", "opencode")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "step_one")
}

func TestE2E_CancelAndRestart(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/e2e-test.yaml", testWorkflow)

	result = runArgus(t, projectDir, "workflow", "start", "e2e-test")
	requireOK(t, result)

	result = runArgus(t, projectDir, "workflow", "cancel")
	data := requireOK(t, result)
	cancelled := data["cancelled"].([]any)
	require.Len(t, cancelled, 1)

	result = runArgus(t, projectDir, "status")
	data = requireOK(t, result)
	assert.Nil(t, data["pipeline"])

	result = runArgus(t, projectDir, "workflow", "start", "e2e-test")
	data = requireOK(t, result)
	assert.Equal(t, "running", data["pipeline_status"])
	assert.Equal(t, "1/3", data["progress"])
}

func TestE2E_FailedJob(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/e2e-test.yaml", testWorkflow)

	result = runArgus(t, projectDir, "workflow", "start", "e2e-test")
	requireOK(t, result)

	result = runArgus(t, projectDir, "job-done", "--fail", "--message", "compilation error")
	data := requireOK(t, result)
	assert.Equal(t, "failed", data["pipeline_status"])
	assert.Equal(t, "step_one", data["failed_job"])
	assert.Nil(t, data["next_job"])

	result = runArgus(t, projectDir, "status")
	data = requireOK(t, result)
	assert.Nil(t, data["pipeline"])
}

func TestE2E_EarlyExit(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/e2e-test.yaml", testWorkflow)

	result = runArgus(t, projectDir, "workflow", "start", "e2e-test")
	requireOK(t, result)

	result = runArgus(t, projectDir, "job-done", "--end-pipeline", "--message", "done early")
	data := requireOK(t, result)
	assert.Equal(t, "completed", data["pipeline_status"])
	assert.Equal(t, true, data["early_exit"])
	assert.Nil(t, data["next_job"])
}

func TestE2E_StatusWithInvariants(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/e2e-test.yaml", testWorkflow)
	passingInv := `version: v0.1.0
id: e2e-pass-inv
description: Always passes
auto: always
check:
  - shell: "exit 0"
    description: "always passes"
prompt: "fix it"
`
	writeFile(t, projectDir, ".argus/invariants/e2e-pass-inv.yaml", passingInv)

	result = runArgus(t, projectDir, "workflow", "start", "e2e-test")
	requireOK(t, result)

	result = runArgus(t, projectDir, "job-done", "--message", "done")
	requireOK(t, result)

	result = runArgus(t, projectDir, "status")
	data := requireOK(t, result)

	pipeline := data["pipeline"].(map[string]any)
	assert.Equal(t, "running", pipeline["status"])

	invariants := data["invariants"].(map[string]any)
	details := invariants["details"].([]any)
	foundPass := false
	for _, d := range details {
		detail := d.(map[string]any)
		if detail["id"] == "e2e-pass-inv" {
			assert.Equal(t, "passed", detail["status"])
			foundPass = true
		}
	}
	assert.True(t, foundPass, "e2e-pass-inv should appear in status details")
	passed := invariants["passed"].(float64)
	assert.GreaterOrEqual(t, passed, float64(1), "at least our custom invariant should pass")
}

func TestE2E_DoctorInInstalledProject(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	result = runArgusText(t, projectDir, "doctor")
	assert.Contains(t, result.Stdout, "PASS")
}

func TestE2E_TrapAlwaysAllows(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgusWithStdin(t, projectDir, `{"tool":"bash","input":"ls"}`, "trap", "--agent", "claude-code")
	require.Equal(t, 0, result.ExitCode)

	data := parseJSON(t, result.Stdout)
	hookOutput := data["hookSpecificOutput"].(map[string]any)
	assert.Equal(t, "allow", hookOutput["permissionDecision"])
}

func TestE2E_Snooze(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)
	sessionID := newDefaultSessionID(t, "e2e-snooze")

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/e2e-test.yaml", testWorkflow)

	result = runArgus(t, projectDir, "workflow", "start", "e2e-test")
	requireOK(t, result)

	result = runArgus(t, projectDir, "workflow", "snooze", "--session", sessionID)
	data := requireOK(t, result)
	snoozed := data["snoozed"].([]any)
	require.Len(t, snoozed, 1)

	stdinJSON := fmt.Sprintf(`{"session_id":"%s","cwd":"%s"}`, sessionID, projectDir)
	result = runArgusWithStdin(t, projectDir, stdinJSON, "tick", "--agent", "claude-code")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "No active pipeline")
	assert.NotContains(t, result.Stdout, "step_one")
}

func TestE2E_DefaultTextOutput(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupGitRepo(t)

	result := runArgus(t, projectDir, "install", "--yes")
	requireOK(t, result)

	writeFile(t, projectDir, ".argus/workflows/e2e-test.yaml", testWorkflow)

	result = runArgusText(t, projectDir, "workflow", "start", "e2e-test")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "Argus: Pipeline")
	assert.Contains(t, result.Stdout, "started (1/3)")
	assert.Contains(t, result.Stdout, "Current job: step_one")

	result = runArgusText(t, projectDir, "job-done", "--message", "done")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "Argus:")
}

func TestE2E_ToolboxCommands(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := t.TempDir()

	tsFile := filepath.Join(projectDir, "test-ts.txt")
	result := runArgus(t, projectDir, "toolbox", "touch-timestamp", tsFile)
	require.Equal(t, 0, result.ExitCode)
	content, err := os.ReadFile(tsFile)
	require.NoError(t, err)
	assert.Regexp(t, `^\d{8}T\d{6}Z$`, string(content))

	result = runArgusWithStdin(t, projectDir, `{"key":"value"}`, "toolbox", "jq", ".key")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "value")

	result = runArgusWithStdin(t, projectDir, "key: value\n", "toolbox", "yq", ".key")
	require.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "value")
}
