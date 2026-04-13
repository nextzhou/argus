package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/sessiontest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tickLifecycleWorkflow = `version: v0.1.0
id: tick-lifecycle
description: Test workflow for tick lifecycle tests
jobs:
  - id: step_one
    prompt: "Do step one"
  - id: step_two
    prompt: "Do step two"
  - id: step_three
    prompt: "Do step three"
`

const tickSessionStartInvariant = `version: v0.1.0
id: tick-session-start-inv
order: 20
description: Session start invariant
auto: session_start
check:
  - shell: "exit 1"
    description: "always fails for testing"
prompt: "Fix the session start issue"
`

const tickAlwaysInvariant = `version: v0.1.0
id: tick-always-inv
order: 10
description: Always invariant
auto: always
check:
  - shell: "exit 1"
    description: "always fails for testing"
prompt: "Fix the always issue"
`

// TestTickLifecycle_Complete verifies the full pipeline lifecycle through tick:
// start → tick (full) → job-done → tick (full, new job) → job-done x2 → tick (no pipeline).
func TestTickLifecycle_Complete(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-lifecycle")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	// Step 1: Start pipeline
	out, err := executeStartCmd(t, "tick-lifecycle")
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, json.Unmarshal(out, &data))
	assert.Equal(t, "ok", data["status"])
	assert.Equal(t, "running", data["pipeline_status"])

	// Step 2: First tick — full context with step-one
	out, err = executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	output := string(out)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "step_one")
	assert.Contains(t, output, "Do step one")
	assert.Contains(t, output, "Current Job:")
	assert.Contains(t, output, "argus job-done")

	// Step 3: job-done step_one → advance to step_two
	out, err = executeJobDoneCmd(t, "--message", "step one done")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(out, &data))
	assert.Equal(t, "running", data["pipeline_status"])

	// Step 4: Tick — full context with NEW job step-two (state changed)
	out, err = executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	output = string(out)
	assert.Contains(t, output, "step_two")
	assert.Contains(t, output, "Do step two")
	assert.Contains(t, output, "Current Job:")
	assert.Contains(t, output, "argus job-done")

	// Step 5: job-done step_two → advance to step_three
	out, err = executeJobDoneCmd(t)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(out, &data))
	assert.Equal(t, "running", data["pipeline_status"])

	// Step 6: job-done step_three → pipeline completed
	out, err = executeJobDoneCmd(t)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(out, &data))
	assert.Equal(t, "completed", data["pipeline_status"])

	// Step 7: Tick after completion — no active pipeline
	out, err = executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	output = string(out)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "argus workflow start")
}

// TestTickLifecycle_MinimalSummary verifies that a repeated tick on unchanged state
// produces a shorter minimal summary instead of the full context.
func TestTickLifecycle_MinimalSummary(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-minimal")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	// Start pipeline
	_, err := executeStartCmd(t, "tick-lifecycle")
	require.NoError(t, err)

	// First tick — full context (state changed: new session)
	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	fullOutput := string(out)
	assert.Contains(t, fullOutput, "Current Job:")
	assert.Contains(t, fullOutput, "Do step one")

	// Second tick — minimal summary (state unchanged)
	out, err = executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	minimalOutput := string(out)
	assert.Contains(t, minimalOutput, "step_one")
	assert.Contains(t, minimalOutput, "argus job-done")
	assert.NotContains(t, minimalOutput, "Current Job:")
	assert.NotContains(t, minimalOutput, "Do step one")

	// Minimal output should be shorter than full context
	assert.Less(t, len(minimalOutput), len(fullOutput),
		"minimal summary (%d bytes) should be shorter than full context (%d bytes)",
		len(minimalOutput), len(fullOutput))
}

// TestTickLifecycle_Snooze verifies that snoozing a pipeline makes tick
// produce dedicated snooze guidance instead of job context.
func TestTickLifecycle_Snooze(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-snooze")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	// Start pipeline
	_, err := executeStartCmd(t, "tick-lifecycle")
	require.NoError(t, err)

	// First tick — full context
	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	assert.Contains(t, string(out), "step_one")

	// Snooze the pipeline
	out, err = executeWorkflowSnoozeCmd(t, store, "--session", sessionID)
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, json.Unmarshal(out, &data))
	assert.Equal(t, "ok", data["status"])
	snoozed := mustJSONArray(t, data["snoozed"])
	require.Len(t, snoozed, 1)

	// Tick after snooze — shows dedicated snooze guidance, no pipeline context
	out, err = executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	output := string(out)
	assertHookSafeTickText(t, output)
	assert.NotContains(t, output, "Current Job:")
	assert.NotContains(t, output, "argus job-done")
}

// TestTickLifecycle_FirstTickInvariant verifies that no-pipeline invariant checks
// stop at the first failure and therefore produce invariant-only output.
func TestTickLifecycle_FirstTickInvariant(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	writeInvariantFixture(t, "tick-session-start-inv", tickSessionStartInvariant)
	writeInvariantFixture(t, "tick-always-inv", tickAlwaysInvariant)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-first-invariant")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	// First tick — stop at the first failing invariant.
	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	output := string(out)
	assert.Contains(t, output, "tick-always-inv")
	assert.Contains(t, output, "Invariant check failed")
	assert.NotContains(t, output, "tick-session-start-inv")
	assert.NotContains(t, output, "No active pipeline")

	// Second tick — session_start remains skipped and always invariant still fails first.
	out, err = executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	output = string(out)
	assert.Contains(t, output, "tick-always-inv")
	assert.NotContains(t, output, "tick-session-start-inv")
}

func TestTickLifecycle_PromptOnlyInvariantRemediation(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	writeInvariantFixture(t, "tick-prompt-only-inv", `version: v0.1.0
id: tick-prompt-only-inv
order: 10
description: Prompt-only invariant
auto: always
check:
  - shell: "exit 1"
    description: "always fails"
prompt: "<<<ARGUS_INIT_REQUIRED>>> initialize argus first"
`)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-prompt-only")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "Invariant check failed")
	assert.Contains(t, output, "tick-prompt-only-inv")
	assert.Contains(t, output, "<<<ARGUS_INIT_REQUIRED>>> initialize argus first")
	assert.NotContains(t, output, "Primary workflow:")
}

func TestTickLifecycle_WorkflowOnlyInvariantRemediation(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	writeInvariantFixture(t, "tick-workflow-only-inv", `version: v0.1.0
id: tick-workflow-only-inv
order: 10
description: Workflow-only invariant
auto: always
check:
  - shell: "exit 1"
    description: "always fails"
workflow: remediation-flow
`)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-workflow-only")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "Invariant check failed")
	assert.Contains(t, output, "tick-workflow-only-inv")
	assert.Contains(t, output, "argus workflow start remediation-flow")
	assert.NotContains(t, output, "<<<ARGUS_INIT_REQUIRED>>>")
}

func TestTickLifecycle_PromptAndWorkflowAreBothRendered(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	writeInvariantFixture(t, "tick-workflow-priority-inv", `version: v0.1.0
id: tick-workflow-priority-inv
order: 10
description: Workflow priority invariant
auto: always
check:
  - shell: "exit 1"
    description: "always fails"
workflow: preferred-remediation
prompt: "<<<ARGUS_INIT_REQUIRED>>> initialize argus first"
`)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-priority")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "Invariant check failed")
	assert.Contains(t, output, "tick-workflow-priority-inv")
	assert.Contains(t, output, "<<<ARGUS_INIT_REQUIRED>>> initialize argus first")
	assert.Contains(t, output, "argus workflow start preferred-remediation")
	assert.Less(t, strings.Index(output, "<<<ARGUS_INIT_REQUIRED>>> initialize argus first"), strings.Index(output, "argus workflow start preferred-remediation"))
}

func TestTickLifecycle_PassingInvariantDoesNotAppendFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	writeInvariantFixture(t, "tick-pass-inv", `version: v0.1.0
id: tick-pass-inv
order: 10
description: Passing invariant
auto: always
check:
  - shell: "exit 0"
    description: "always passes"
prompt: "this prompt should never be injected"
`)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-pass")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "No active pipeline")
	assert.NotContains(t, output, "Invariant check failed")
	assert.NotContains(t, output, "tick-pass-inv")
	assert.NotContains(t, output, "this prompt should never be injected")
}

func TestTickLifecycle_ActivePipelineSkipsInvariantChecks(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	writeInvariantFixture(t, "tick-active-pipeline-inv", `version: v0.1.0
id: tick-active-pipeline-inv
order: 10
description: Should be skipped while pipeline is active
auto: always
check:
  - shell: "exit 1"
    description: "always fails"
prompt: "do not show this while a pipeline is active"
`)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-active-pipeline")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	_, err := executeStartCmd(t, "tick-lifecycle")
	require.NoError(t, err)

	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "Current Job:")
	assert.Contains(t, output, "step_one")
	assert.NotContains(t, output, "Invariant check failed")
	assert.NotContains(t, output, "tick-active-pipeline-inv")
	assert.NotContains(t, output, "do not show this while a pipeline is active")
}

func TestTickLifecycle_NoWorkflowAndPassingInvariantReturnsEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	writeInvariantFixture(t, "tick-pass-inv", `version: v0.1.0
id: tick-pass-inv
order: 10
description: Passing invariant
auto: always
check:
  - shell: "exit 0"
    description: "always passes"
prompt: "this prompt should never be injected"
`)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-no-workflow")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	assert.Empty(t, string(out))
}

// TestTickLifecycle_SubAgentSkip verifies that sub-agent ticks produce
// empty output even when an active pipeline exists.
func TestTickLifecycle_SubAgentSkip(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	store := sessiontest.NewMemoryStore()

	// Start pipeline so there is active work
	_, err := executeStartCmd(t, "tick-lifecycle")
	require.NoError(t, err)

	sessionID := sessiontest.NewSessionID(t, "tick-sub-agent")
	// Sub-agent: include agent_id in stdin JSON
	stdinJSON := fmt.Sprintf(`{"session_id":"%s","agent_id":"worker-1"}`, sessionID)

	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)
	assert.Empty(t, string(out))
}

// TestTickLifecycle_InvariantStatusIntegration verifies that the status command
// shows both pipeline progress and invariant results in a single output.
func TestTickLifecycle_InvariantStatusIntegration(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	writeInvariantFixture(t, "tick-status-inv", `version: v0.1.0
id: tick-status-inv
order: 10
description: Status integration invariant
auto: always
check:
  - shell: "exit 0"
    description: "always passes"
prompt: "Fix it"
`)

	// Step 1: Start pipeline
	out, err := executeStartCmd(t, "tick-lifecycle")
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, json.Unmarshal(out, &data))
	assert.Equal(t, "ok", data["status"])
	assert.Equal(t, "running", data["pipeline_status"])

	// Step 2: Advance one job
	out, err = executeJobDoneCmd(t)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(out, &data))
	assert.Equal(t, "running", data["pipeline_status"])

	// Step 3: Status shows both pipeline progress and invariant info
	out, err = executeStatusCmd(t)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(out, &data))
	assert.Equal(t, "ok", data["status"])

	// Verify pipeline info
	p := mustJSONObject(t, data["pipeline"])
	assert.Equal(t, "tick-lifecycle", p["workflow_id"])
	assert.Equal(t, "running", p["status"])
	progress := mustJSONObject(t, p["progress"])
	assert.InDelta(t, 2, progress["current"], 0)
	assert.InDelta(t, 3, progress["total"], 0)

	// Verify invariant info
	inv := mustJSONObject(t, data["invariants"])
	assert.InDelta(t, 1, inv["passed"], 0)
	assert.InDelta(t, 0, inv["failed"], 0)
	details := mustJSONArray(t, inv["details"])
	require.Len(t, details, 1)
	d0 := mustJSONObject(t, details[0])
	assert.Equal(t, "tick-status-inv", d0["id"])
	assert.Equal(t, "passed", d0["status"])
}

func TestTickLifecycle_FirstTickUsesGlobalOrderAcrossAutoModes(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "tick-lifecycle", tickLifecycleWorkflow)
	writeInvariantFixture(t, "tick-session-start-inv", `version: v0.1.0
id: tick-session-start-inv
order: 10
description: Session start invariant
auto: session_start
check:
  - shell: "exit 1"
    description: "always fails for testing"
prompt: "Fix the session start issue"
`)
	writeInvariantFixture(t, "tick-always-inv", `version: v0.1.0
id: tick-always-inv
order: 20
description: Always invariant
auto: always
check:
  - shell: "exit 1"
    description: "always fails for testing"
prompt: "Fix the always issue"
`)
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "tick-order-across-auto")
	stdinJSON := fmt.Sprintf(`{"session_id":"%s"}`, sessionID)

	out, err := executeTickCmd(t, store, stdinJSON, "--agent", "claude-code")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "tick-session-start-inv")
	assert.NotContains(t, output, "tick-always-inv")
}
