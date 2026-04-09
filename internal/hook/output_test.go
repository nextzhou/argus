package hook

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertHookSafeTickText(t *testing.T, output string) {
	t.Helper()

	trimmed := strings.TrimLeft(output, " \t\r\n")
	require.NotEmpty(t, trimmed)
	assert.NotEqual(t, '[', rune(trimmed[0]))
	assert.NotEqual(t, '{', rune(trimmed[0]))
}

func TestFormatNoPipeline(t *testing.T) {
	workflows := []WorkflowSummary{
		{ID: "release", Description: "Release workflow"},
		{ID: "argus-init", Description: "Initialize Argus config"},
	}

	output, err := FormatNoPipeline(workflows)

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "Argus:")
	assert.Contains(t, output, "No active pipeline")
	assert.Contains(t, output, "release")
	assert.Contains(t, output, "argus-init")
	assert.Contains(t, output, "argus workflow start")
}

func TestFormatNoPipeline_Empty(t *testing.T) {
	output, err := FormatNoPipeline([]WorkflowSummary{})

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "Argus:")
	assert.Contains(t, output, "No active pipeline")
	assert.Contains(t, output, "argus workflow start")
	assert.Contains(t, output, "(none)")
}

func TestFormatFullContext(t *testing.T) {
	output, err := FormatFullContext(
		"release-20240405T103000Z",
		"release",
		"2/5",
		"run_tests",
		"Run all tests and ensure they pass",
		"argus-run-tests",
		"ses_abc123",
	)

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "Argus:")
	assert.Contains(t, output, "release-20240405T103000Z")
	assert.Contains(t, output, "release")
	assert.Contains(t, output, "2/5")
	assert.Contains(t, output, "run_tests")
	assert.Contains(t, output, "Run all tests and ensure they pass")
	assert.Contains(t, output, "argus-run-tests")
	assert.Contains(t, output, "argus job-done")
	assert.Contains(t, output, "argus workflow snooze")
	assert.Contains(t, output, "ses_abc123")
	assert.Contains(t, output, "argus workflow cancel")
}

func TestFormatFullContext_WithSkill(t *testing.T) {
	output, err := FormatFullContext(
		"pipeline-1",
		"workflow-1",
		"1/3",
		"job-1",
		"Do something",
		"my-skill",
		"session-1",
	)

	require.NoError(t, err)
	assert.Contains(t, output, "Skill: my-skill")
}

func TestFormatFullContext_NoSkill(t *testing.T) {
	output, err := FormatFullContext(
		"pipeline-1",
		"workflow-1",
		"1/3",
		"job-1",
		"Do something",
		"",
		"session-1",
	)

	require.NoError(t, err)
	assert.NotContains(t, output, "Skill:")
}

func TestFormatMinimalSummary(t *testing.T) {
	output, err := FormatMinimalSummary("release", "run_tests", "2/5")

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "Argus:")
	assert.Contains(t, output, "release")
	assert.Contains(t, output, "run_tests")
	assert.Contains(t, output, "2/5")
	assert.Contains(t, output, "argus job-done")
}

func TestFormatSnoozed(t *testing.T) {
	workflows := []WorkflowSummary{
		{ID: "release", Description: "Release workflow"},
	}

	output, err := FormatSnoozed(workflows)

	require.NoError(t, err)
	// Should be identical to FormatNoPipeline
	expected, err := FormatNoPipeline(workflows)
	require.NoError(t, err)
	assert.Equal(t, expected, output)
}

func TestAppendInvariantFailed(t *testing.T) {
	base := "Argus: Some base output"
	failures := []InvariantFailure{
		{
			ID:          "argus-init",
			Description: "Project not initialized",
			Suggestion:  "Run argus-init workflow",
		},
	}

	output, err := AppendInvariantFailed(base, failures)

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, base)
	assert.Contains(t, output, "---")
	assert.Contains(t, output, "Argus: Invariant check failed:")
	assert.Contains(t, output, "argus-init")
	assert.Contains(t, output, "Project not initialized")
	assert.Contains(t, output, "Suggestion: Run argus-init workflow")
}

func TestAppendInvariantFailed_Empty(t *testing.T) {
	base := "Argus: Some base output"

	output, err := AppendInvariantFailed(base, []InvariantFailure{})

	require.NoError(t, err)
	assert.Equal(t, base, output)
}

func TestAppendInvariantFailed_Multiple(t *testing.T) {
	base := "Argus: Base"
	failures := []InvariantFailure{
		{
			ID:          "check-1",
			Description: "First check failed",
			Suggestion:  "Fix first",
		},
		{
			ID:          "check-2",
			Description: "Second check failed",
			Suggestion:  "Fix second",
		},
	}

	output, err := AppendInvariantFailed(base, failures)

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "check-1")
	assert.Contains(t, output, "check-2")
	assert.Contains(t, output, "First check failed")
	assert.Contains(t, output, "Second check failed")
}
