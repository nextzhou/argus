package hook

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleTick_NoPipeline(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)

	sessionBaseDir := t.TempDir()

	var out bytes.Buffer
	err := HandleTick(
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-no-pipeline","cwd":"`+projectRoot+`"}`),
		&out,
		projectRoot,
		sessionBaseDir,
	)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "[Argus]")
	assert.Contains(t, output, "No active pipeline")
	assert.Contains(t, output, "release")
	assert.Contains(t, output, "argus workflow start")
	assert.True(t, session.Exists(sessionBaseDir, "ses-no-pipeline"))
}

func TestHandleTick_SubAgent(t *testing.T) {
	var out bytes.Buffer
	err := HandleTick(
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-sub-agent","agent_id":"worker-1"}`),
		&out,
		t.TempDir(),
		t.TempDir(),
	)
	require.NoError(t, err)
	assert.Empty(t, out.String())
}

func TestHandleTick_NoProjectRoot(t *testing.T) {
	var out bytes.Buffer
	err := HandleTick(
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-no-root"}`),
		&out,
		t.TempDir(),
		t.TempDir(),
	)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "[Argus] Warning")
	assert.Contains(t, out.String(), "not inside an Argus project")
}

func TestInvariantSuggestion(t *testing.T) {
	tests := []struct {
		name string
		inv  *invariant.Invariant
		want string
	}{
		{
			name: "nil invariant falls back to generic guidance",
			inv:  nil,
			want: "Review the invariant definition and project state",
		},
		{
			name: "workflow takes priority over prompt",
			inv: &invariant.Invariant{
				Workflow: "argus-init",
				Prompt:   "<<<ARGUS_INIT_REQUIRED>>>",
			},
			want: "Run argus workflow start argus-init",
		},
		{
			name: "prompt is used when workflow is absent",
			inv: &invariant.Invariant{
				Prompt: "<<<ARGUS_INIT_REQUIRED>>>",
			},
			want: "<<<ARGUS_INIT_REQUIRED>>>",
		},
		{
			name: "empty invariant falls back to generic guidance",
			inv:  &invariant.Invariant{},
			want: "Review the invariant definition and project state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, invariantSuggestion(tt.inv))
		})
	}
}

func TestBuildTickOutput_NoActivePipeline(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)

	output, logDetails, snapshotPipelineID, snapshotJobID := buildTickOutput(projectRoot, "ses-no-pipeline", &session.Session{}, nil, nil)

	assert.Equal(t, FormatNoPipeline([]WorkflowSummary{{ID: "release", Description: "Release workflow"}}), output)
	assert.Contains(t, logDetails, "active=0")
	assert.Contains(t, logDetails, "warnings=0")
	assert.Contains(t, logDetails, "scenario=no-pipeline")
	assert.Empty(t, snapshotPipelineID)
	assert.Empty(t, snapshotJobID)
}

func TestBuildTickOutput_SnoozedPipeline(t *testing.T) {
	projectRoot := t.TempDir()
	instanceID := "release-20240115T103000Z"
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)
	writeTickPipelineFixture(t, projectRoot, instanceID, `version: v0.1.0
workflow_id: release
status: running
current_job: run_tests
started_at: "20240115T103000Z"
jobs:
  run_tests:
    started_at: "20240115T103000Z"
`)

	activePipelines, scanWarnings := loadTickActivePipelines(t, projectRoot)
	output, logDetails, snapshotPipelineID, snapshotJobID := buildTickOutput(projectRoot, "ses-snoozed", &session.Session{SnoozedPipelines: []string{instanceID}}, activePipelines, scanWarnings)

	assert.Equal(t, FormatSnoozed([]WorkflowSummary{{ID: "release", Description: "Release workflow"}}), output)
	assert.Contains(t, logDetails, "scenario=snoozed")
	assert.Empty(t, snapshotPipelineID)
	assert.Empty(t, snapshotJobID)
}

func TestBuildTickOutput_MissingCurrentJob(t *testing.T) {
	projectRoot := t.TempDir()
	instanceID := "release-20240115T103000Z"
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)
	writeTickPipelineFixture(t, projectRoot, instanceID, `version: v0.1.0
workflow_id: release
status: running
current_job: null
started_at: "20240115T103000Z"
jobs: {}
`)

	activePipelines, scanWarnings := loadTickActivePipelines(t, projectRoot)
	output, logDetails, snapshotPipelineID, snapshotJobID := buildTickOutput(projectRoot, "ses-missing-job", &session.Session{}, activePipelines, scanWarnings)

	assert.Contains(t, output, "[Argus] No active pipeline.")
	assert.Contains(t, output, "active pipeline is missing current job state")
	assert.Contains(t, logDetails, "scenario=missing-current-job")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Empty(t, snapshotJobID)
}

func TestBuildTickOutput_WorkflowLoadFailure(t *testing.T) {
	projectRoot := t.TempDir()
	instanceID := "missing-20240115T103000Z"
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)
	writeTickPipelineFixture(t, projectRoot, instanceID, `version: v0.1.0
workflow_id: missing
status: running
current_job: run_tests
started_at: "20240115T103000Z"
jobs:
  run_tests:
    started_at: "20240115T103000Z"
`)

	activePipelines, scanWarnings := loadTickActivePipelines(t, projectRoot)
	output, logDetails, snapshotPipelineID, snapshotJobID := buildTickOutput(projectRoot, "ses-missing-workflow", &session.Session{}, activePipelines, scanWarnings)

	assert.Contains(t, output, "[Argus] No active pipeline.")
	assert.Contains(t, output, "could not load workflow missing")
	assert.Contains(t, logDetails, "scenario=workflow-load-error")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Equal(t, "run_tests", snapshotJobID)
}

func TestBuildTickOutput_JobNotFoundInWorkflow(t *testing.T) {
	projectRoot := t.TempDir()
	instanceID := "release-20240115T103000Z"
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)
	writeTickPipelineFixture(t, projectRoot, instanceID, `version: v0.1.0
workflow_id: release
status: running
current_job: deploy
started_at: "20240115T103000Z"
jobs:
  deploy:
    started_at: "20240115T103000Z"
`)

	activePipelines, scanWarnings := loadTickActivePipelines(t, projectRoot)
	output, logDetails, snapshotPipelineID, snapshotJobID := buildTickOutput(projectRoot, "ses-workflow-mismatch", &session.Session{}, activePipelines, scanWarnings)

	assert.Contains(t, output, "[Argus] No active pipeline.")
	assert.Contains(t, output, "current job deploy was not found in workflow release")
	assert.Contains(t, logDetails, "scenario=workflow-mismatch")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Equal(t, "deploy", snapshotJobID)
}

func TestBuildTickOutput_StateChangeDetection(t *testing.T) {
	projectRoot := t.TempDir()
	instanceID := "release-20240115T103000Z"
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests with context"
    skill: test-skill
`)
	writeTickPipelineFixture(t, projectRoot, instanceID, `version: v0.1.0
workflow_id: release
status: running
current_job: run_tests
started_at: "20240115T103000Z"
jobs:
  run_tests:
    started_at: "20240115T103000Z"
`)

	activePipelines, scanWarnings := loadTickActivePipelines(t, projectRoot)
	sess := &session.Session{}

	fullOutput, fullLogDetails, snapshotPipelineID, snapshotJobID := buildTickOutput(projectRoot, "ses-state-change", sess, activePipelines, scanWarnings)
	assert.Equal(t, FormatFullContext(instanceID, "release", "1/1", "run_tests", "Run tests with context", "test-skill", "ses-state-change"), fullOutput)
	assert.Contains(t, fullLogDetails, "scenario=full")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Equal(t, "run_tests", snapshotJobID)

	sess.LastTick = &session.LastTickState{Pipeline: instanceID, Job: "run_tests"}
	minimalOutput, minimalLogDetails, snapshotPipelineID, snapshotJobID := buildTickOutput(projectRoot, "ses-state-change", sess, activePipelines, scanWarnings)
	assert.Equal(t, FormatMinimalSummary("release", "run_tests", "1/1"), minimalOutput)
	assert.Contains(t, minimalLogDetails, "scenario=minimal")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Equal(t, "run_tests", snapshotJobID)
}

func writeTickWorkflowFixture(t *testing.T, projectRoot, workflowID, yamlContent string) {
	t.Helper()
	workflowsDir := filepath.Join(projectRoot, ".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, workflowID+".yaml"), []byte(yamlContent), 0o644))
}

func writeTickPipelineFixture(t *testing.T, projectRoot, instanceID, yamlContent string) {
	t.Helper()
	pipelinesDir := filepath.Join(projectRoot, ".argus", "pipelines")
	require.NoError(t, os.MkdirAll(pipelinesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pipelinesDir, instanceID+".yaml"), []byte(yamlContent), 0o644))
}

func loadTickActivePipelines(t *testing.T, projectRoot string) ([]pipeline.ActivePipeline, []pipeline.ScanWarning) {
	t.Helper()
	activePipelines, scanWarnings, err := pipeline.ScanActivePipelines(filepath.Join(projectRoot, ".argus", "pipelines"))
	require.NoError(t, err)
	return activePipelines, scanWarnings
}

