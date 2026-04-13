package hook

import (
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/invariant"
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
		{ID: "argus-project-init", Description: "Initialize Argus config"},
	}

	output, err := FormatNoPipeline(workflows)

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "release")
	assert.Contains(t, output, "Release workflow")
	assert.Contains(t, output, "argus-project-init")
	assert.Contains(t, output, "Initialize Argus config")
	assert.Contains(t, output, "argus workflow start")
}

func TestFormatNoPipeline_Empty(t *testing.T) {
	output, err := FormatNoPipeline([]WorkflowSummary{})

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "argus workflow start")
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
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "release")
	assert.Contains(t, output, "Release workflow")
	assert.NotContains(t, output, "argus job-done")
}

func TestFormatInvariantFailure(t *testing.T) {
	exitCode := 1
	output, err := FormatInvariantFailure(InvariantFailure{
		Invariant: &invariant.Invariant{
			ID:          "argus-project-init",
			Description: "Project not initialized",
			Prompt:      "Run `argus setup --yes` to initialize project-level Argus.",
			Workflow:    "argus-project-init",
		},
		FailedStep: &invariant.StepResult{
			Check: invariant.CheckStep{
				Description: "Project-level Argus directory exists",
				Shell:       "test -d .argus",
			},
			Status:      "fail",
			ExitCode:    &exitCode,
			FailureKind: "exit",
		},
	})

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "argus-project-init")
	assert.Contains(t, output, "Project not initialized")
	assert.Contains(t, output, "Project-level Argus directory exists")
	assert.Contains(t, output, "test -d .argus")
	assert.Contains(t, output, "exited with code 1")
	assert.Contains(t, output, "Run `argus setup --yes` to initialize project-level Argus.")
	assert.Contains(t, output, "argus workflow start argus-project-init")
	assert.NotContains(t, output, "argus job-done")
	assert.NotContains(t, output, "---")
}

func TestFormatActivePipelineIssue(t *testing.T) {
	output, err := FormatActivePipelineIssue(ActivePipelineIssue{
		PipelineID:          "release-20240405T103000Z",
		WorkflowID:          "release",
		Issue:               "current job deploy was not found in workflow release",
		InvestigateCommand:  "argus status",
		InvestigateGuidance: "inspect the current pipeline state",
		SessionID:           "ses-123",
	})

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "release-20240405T103000Z")
	assert.Contains(t, output, "release")
	assert.Contains(t, output, "current job deploy was not found in workflow release")
	assert.Contains(t, output, "argus status")
	assert.Contains(t, output, "argus workflow cancel")
	assert.Contains(t, output, "argus workflow snooze --session ses-123")
}

func TestFormatMultipleActivePipelines(t *testing.T) {
	output, err := FormatMultipleActivePipelines([]string{
		"release-20240405T103000Z",
		"hotfix-20240405T104500Z",
	}, "ses-456")

	require.NoError(t, err)
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "release-20240405T103000Z")
	assert.Contains(t, output, "hotfix-20240405T104500Z")
	assert.Contains(t, output, "argus workflow cancel")
	assert.Contains(t, output, "argus workflow snooze --session ses-456")
	assert.Contains(t, output, "argus doctor")
}
