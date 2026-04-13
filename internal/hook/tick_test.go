package hook

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/scope"
	"github.com/nextzhou/argus/internal/session"
	"github.com/nextzhou/argus/internal/workflow"
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
		context.Background(),
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-no-pipeline","cwd":"`+projectRoot+`"}`),
		&out,
		projectRoot,
		sessionBaseDir,
	)
	require.NoError(t, err)

	output := out.String()
	expected, err := FormatNoPipeline([]WorkflowSummary{{ID: "release", Description: "Release workflow"}})
	require.NoError(t, err)
	assert.Equal(t, expected, output)
	assert.True(t, session.Exists(sessionBaseDir, "ses-no-pipeline"))
}

func TestHandleTick_SubAgent(t *testing.T) {
	var out bytes.Buffer
	err := HandleTick(
		context.Background(),
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
		context.Background(),
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-no-root"}`),
		&out,
		t.TempDir(),
		t.TempDir(),
	)
	require.NoError(t, err)
	assertHookSafeTickText(t, out.String())
	assert.Contains(t, out.String(), "Argus warning")
	assert.Contains(t, out.String(), "not inside an Argus project")
}

func TestHandleTick_MultipleActivePipelines(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickPipelineFixture(t, projectRoot, "release-20240115T103000Z", `version: v0.1.0
workflow_id: release
status: running
current_job: run_tests
started_at: "20240115T103000Z"
jobs:
  run_tests:
    started_at: "20240115T103000Z"
`)
	writeTickPipelineFixture(t, projectRoot, "hotfix-20240115T104500Z", `version: v0.1.0
workflow_id: hotfix
status: running
current_job: verify
started_at: "20240115T104500Z"
jobs:
  verify:
    started_at: "20240115T104500Z"
`)

	var out bytes.Buffer
	err := HandleTick(
		context.Background(),
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-multi-active","cwd":"`+projectRoot+`"}`),
		&out,
		projectRoot,
		t.TempDir(),
	)
	require.NoError(t, err)
	expected, formatErr := FormatMultipleActivePipelines([]string{
		"hotfix-20240115T104500Z",
		"release-20240115T103000Z",
	}, "ses-multi-active")
	require.NoError(t, formatErr)
	assert.Equal(t, expected, out.String())
}

func TestLoadTickWorkflowSummaries_LoadError(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".argus"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".argus", "workflows"), []byte("not a dir"), 0o600))

	logs := captureDebugLogs(t, func() {
		summaries := loadTickWorkflowSummaries(scope.NewProjectScope(projectRoot))
		assert.Nil(t, summaries)
	})

	assert.Contains(t, logs, "tick: could not load workflow summaries")
	assert.Contains(t, logs, "reading workflows directory")
}

func TestRunTickInvariants_LoadError(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".argus"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".argus", "invariants"), []byte("not a dir"), 0o600))

	logs := captureDebugLogs(t, func() {
		catalog, warning := loadTickInvariantCatalog(scope.NewProjectScope(projectRoot))
		assert.Empty(t, catalog.Invariants)
		assert.Contains(t, warning, "could not load invariants")
	})

	assert.Contains(t, logs, "tick: could not load invariants")
	assert.Contains(t, logs, "loading invariant catalog")
}

func TestShouldRunInvariantAuto(t *testing.T) {
	tests := []struct {
		name      string
		inv       *invariant.Invariant
		firstTick bool
		want      bool
	}{
		{
			name:      "nil invariant is skipped",
			inv:       nil,
			firstTick: true,
			want:      false,
		},
		{
			name:      "missing auto is skipped on first tick",
			inv:       &invariant.Invariant{},
			firstTick: true,
			want:      false,
		},
		{
			name:      "always runs on first tick",
			inv:       &invariant.Invariant{Auto: "always"},
			firstTick: true,
			want:      true,
		},
		{
			name:      "session start runs on first tick",
			inv:       &invariant.Invariant{Auto: "session_start"},
			firstTick: true,
			want:      true,
		},
		{
			name:      "always runs after first tick",
			inv:       &invariant.Invariant{Auto: "always"},
			firstTick: false,
			want:      true,
		},
		{
			name:      "session start is skipped after first tick",
			inv:       &invariant.Invariant{Auto: "session_start"},
			firstTick: false,
			want:      false,
		},
		{
			name:      "never is skipped",
			inv:       &invariant.Invariant{Auto: "never"},
			firstTick: true,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldRunInvariantAuto(tt.inv, tt.firstTick))
		})
	}
}

func TestAppendTickWarningText(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		warning string
		want    string
	}{
		{
			name:    "empty base with warning",
			base:    "",
			warning: "watch out",
			want:    "Argus warning: watch out\n",
		},
		{
			name:    "base with empty warning",
			base:    "base text",
			warning: "",
			want:    "base text",
		},
		{
			name:    "base without trailing newline",
			base:    "base text",
			warning: "watch out",
			want:    "base text\nArgus warning: watch out\n",
		},
		{
			name:    "base with trailing newline",
			base:    "base text\n",
			warning: "watch out",
			want:    "base text\nArgus warning: watch out\n",
		},
		{
			name:    "both empty",
			base:    "",
			warning: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, appendTickWarningText(tt.base, tt.warning))
		})
	}
}

func TestBuildPipelineJobDataMap(t *testing.T) {
	messageOne := "Preparation complete"
	messageTwo := "Tests passed"
	endedAtOne := "20240115T103000Z"
	endedAtTwo := "20240115T103500Z"

	tests := []struct {
		name string
		p    *pipeline.Pipeline
		want map[string]*workflow.PipelineJobData
	}{
		{
			name: "nil pipeline returns empty map",
			p:    nil,
			want: map[string]*workflow.PipelineJobData{},
		},
		{
			name: "empty jobs returns empty map",
			p:    &pipeline.Pipeline{Jobs: map[string]*pipeline.JobData{}},
			want: map[string]*workflow.PipelineJobData{},
		},
		{
			name: "nil job data entries are skipped",
			p: &pipeline.Pipeline{Jobs: map[string]*pipeline.JobData{
				"prepare": nil,
				"deploy":  &pipeline.JobData{StartedAt: "20240115T103100Z"},
			}},
			want: map[string]*workflow.PipelineJobData{
				"deploy": &workflow.PipelineJobData{StartedAt: "20240115T103100Z"},
			},
		},
		{
			name: "messages and timestamps are copied for multiple jobs",
			p: &pipeline.Pipeline{Jobs: map[string]*pipeline.JobData{
				"prepare": &pipeline.JobData{
					StartedAt: "20240115T102000Z",
					EndedAt:   &endedAtOne,
					Message:   &messageOne,
				},
				"run_tests": &pipeline.JobData{
					StartedAt: "20240115T103100Z",
					EndedAt:   &endedAtTwo,
					Message:   &messageTwo,
				},
			}},
			want: map[string]*workflow.PipelineJobData{
				"prepare": &workflow.PipelineJobData{
					StartedAt: "20240115T102000Z",
					EndedAt:   &endedAtOne,
					Message:   &messageOne,
				},
				"run_tests": &workflow.PipelineJobData{
					StartedAt: "20240115T103100Z",
					EndedAt:   &endedAtTwo,
					Message:   &messageTwo,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildPipelineJobDataMap(tt.p))
		})
	}
}

func TestBuildNoActivePipelineOutput_ShowsWorkflowsWhenAvailable(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)

	output, logDetails := buildNoActivePipelineOutput(context.Background(), scope.NewProjectScope(projectRoot), &session.Session{}, false)

	expected, err := FormatNoPipeline([]WorkflowSummary{{ID: "release", Description: "Release workflow"}})
	require.NoError(t, err)
	assert.Equal(t, expected, output)
	assert.Contains(t, logDetails, "active=0")
	assert.Contains(t, logDetails, "scenario=no-pipeline")
}

func TestBuildNoActivePipelineOutput_InvariantFailureIsExclusive(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)
	writeTickInvariantFixture(t, projectRoot, "argus-project-init", `version: v0.1.0
id: argus-project-init
order: 10
description: Project not initialized
auto: always
check:
  - shell: "exit 1"
    description: "init workflow has not been completed"
prompt: "Initialize the project first"
`)

	output, logDetails := buildNoActivePipelineOutput(context.Background(), scope.NewProjectScope(projectRoot), &session.Session{}, false)

	exitCode := 1
	expected, err := FormatInvariantFailure(InvariantFailure{
		Invariant: &invariant.Invariant{
			ID:          "argus-project-init",
			Description: "Project not initialized",
			Prompt:      "Initialize the project first",
		},
		FailedStep: &invariant.StepResult{
			Check: invariant.CheckStep{
				Description: "init workflow has not been completed",
				Shell:       "exit 1",
			},
			Status:      "fail",
			ExitCode:    &exitCode,
			FailureKind: "exit",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, expected, output)
	assert.Contains(t, logDetails, "scenario=invariant-failed")
}

func TestBuildNoActivePipelineOutput_NoWorkflowsReturnsEmpty(t *testing.T) {
	output, logDetails := buildNoActivePipelineOutput(context.Background(), scope.NewProjectScope(t.TempDir()), &session.Session{}, false)

	assert.Empty(t, output)
	assert.Contains(t, logDetails, "active=0")
	assert.Contains(t, logDetails, "scenario=no-output")
}

func TestHandleTick_NoPipelineSlowInvariantWarnsOncePerSession(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)
	writeTickInvariantFixture(t, projectRoot, "slow-check", `version: v0.1.0
id: slow-check
order: 10
description: Slow check
auto: always
check:
  - shell: "sleep 3"
prompt: "Fix it"
`)

	sessionBaseDir := t.TempDir()
	input := bytes.NewBufferString(`{"session_id":"ses-slow-once","cwd":"` + projectRoot + `"}`)

	var firstOut bytes.Buffer
	err := HandleTick(context.Background(), "claude-code", false, input, &firstOut, projectRoot, sessionBaseDir)
	require.NoError(t, err)
	assert.Contains(t, firstOut.String(), "Invariant checks took 3.")
	assert.Contains(t, firstOut.String(), "argus-doctor")
	assert.Contains(t, firstOut.String(), "argus doctor --check-invariants")

	var secondOut bytes.Buffer
	err = HandleTick(
		context.Background(),
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-slow-once","cwd":"`+projectRoot+`"}`),
		&secondOut,
		projectRoot,
		sessionBaseDir,
	)
	require.NoError(t, err)
	assert.NotContains(t, secondOut.String(), "Invariant checks took")
}

func TestHandleTick_NoPipelineWarnsWhenSessionBecomesSlow(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: run_tests
    prompt: "Run tests"
`)
	writeTickInvariantFixture(t, projectRoot, "check", `version: v0.1.0
id: check
order: 10
description: Fast check
auto: always
check:
  - shell: ":"
prompt: "Fix it"
`)

	sessionBaseDir := t.TempDir()

	var firstOut bytes.Buffer
	err := HandleTick(
		context.Background(),
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-slow-later","cwd":"`+projectRoot+`"}`),
		&firstOut,
		projectRoot,
		sessionBaseDir,
	)
	require.NoError(t, err)
	assert.NotContains(t, firstOut.String(), "Invariant checks took")

	writeTickInvariantFixture(t, projectRoot, "check", `version: v0.1.0
id: check
order: 10
description: Slow check
auto: always
check:
  - shell: "sleep 3"
prompt: "Fix it"
`)

	var secondOut bytes.Buffer
	err = HandleTick(
		context.Background(),
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-slow-later","cwd":"`+projectRoot+`"}`),
		&secondOut,
		projectRoot,
		sessionBaseDir,
	)
	require.NoError(t, err)
	assert.Contains(t, secondOut.String(), "Invariant checks took 3.")
}

func TestHandleTick_InvariantFailureSuppressesSlowWarning(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickInvariantFixture(t, projectRoot, "slow-fail", `version: v0.1.0
id: slow-fail
order: 10
description: Slow failure
auto: always
check:
  - shell: "sleep 3; exit 1"
prompt: "Fix the invariant"
`)

	var out bytes.Buffer
	err := HandleTick(
		context.Background(),
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-slow-fail","cwd":"`+projectRoot+`"}`),
		&out,
		projectRoot,
		t.TempDir(),
	)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Invariant check failed")
	assert.NotContains(t, out.String(), "Invariant checks took")
}

func TestBuildActivePipelineOutput_SnoozedPipeline(t *testing.T) {
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
	output, logDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(context.Background(), scope.NewProjectScope(projectRoot), "ses-snoozed", &session.Session{SnoozedPipelines: []string{instanceID}}, activePipelines, scanWarnings)

	expected, err := FormatSnoozed([]WorkflowSummary{{ID: "release", Description: "Release workflow"}})
	require.NoError(t, err)
	assert.Equal(t, expected, output)
	assert.Contains(t, logDetails, "scenario=snoozed")
	assert.Empty(t, snapshotPipelineID)
	assert.Empty(t, snapshotJobID)
}

func TestBuildActivePipelineOutput_MissingCurrentJob(t *testing.T) {
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
	output, logDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(context.Background(), scope.NewProjectScope(projectRoot), "ses-missing-job", &session.Session{}, activePipelines, scanWarnings)

	expected, err := FormatActivePipelineIssue(ActivePipelineIssue{
		PipelineID:          instanceID,
		WorkflowID:          "release",
		Issue:               "The active pipeline is missing current job state.",
		InvestigateCommand:  "argus status",
		InvestigateGuidance: "inspect the current pipeline state",
		SessionID:           "ses-missing-job",
	})
	require.NoError(t, err)
	assert.Equal(t, expected, output)
	assert.Contains(t, logDetails, "scenario=missing-current-job")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Empty(t, snapshotJobID)
}

func TestBuildActivePipelineOutput_WorkflowLoadFailure(t *testing.T) {
	projectRoot := t.TempDir()
	instanceID := "missing-20240115T103000Z"
	writeTickWorkflowFixture(t, projectRoot, "available", `version: v0.1.0
id: available
description: Available workflow
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
	output, logDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(context.Background(), scope.NewProjectScope(projectRoot), "ses-missing-workflow", &session.Session{}, activePipelines, scanWarnings)

	assertHookSafeTickText(t, output)
	assert.Contains(t, output, instanceID)
	assert.Contains(t, output, "missing")
	assert.Contains(t, output, "could not load workflow missing")
	assert.Contains(t, output, filepath.Join(projectRoot, ".argus", "workflows", "missing.yaml"))
	assert.Contains(t, output, "argus doctor")
	assert.Contains(t, output, "argus workflow cancel")
	assert.Contains(t, output, "argus workflow snooze --session ses-missing-workflow")
	assert.Contains(t, logDetails, "scenario=workflow-load-error")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Equal(t, "run_tests", snapshotJobID)
}

func TestBuildActivePipelineOutput_JobNotFoundInWorkflow(t *testing.T) {
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
	output, logDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(context.Background(), scope.NewProjectScope(projectRoot), "ses-workflow-mismatch", &session.Session{}, activePipelines, scanWarnings)

	expected, err := FormatActivePipelineIssue(ActivePipelineIssue{
		PipelineID:          instanceID,
		WorkflowID:          "release",
		Issue:               "current job deploy was not found in workflow release",
		InvestigateCommand:  "argus status",
		InvestigateGuidance: "inspect the current pipeline state before deciding whether to cancel it",
		SessionID:           "ses-workflow-mismatch",
	})
	require.NoError(t, err)
	assert.Equal(t, expected, output)
	assert.Contains(t, logDetails, "scenario=workflow-mismatch")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Equal(t, "deploy", snapshotJobID)
}

func TestBuildActivePipelineOutput_StateChangeDetection(t *testing.T) {
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

	fullOutput, fullLogDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(context.Background(), scope.NewProjectScope(projectRoot), "ses-state-change", sess, activePipelines, scanWarnings)
	expectedFullOutput, err := FormatFullContext(instanceID, "release", "1/1", "run_tests", "Run tests with context", "test-skill", "ses-state-change")
	require.NoError(t, err)
	assert.Equal(t, expectedFullOutput, fullOutput)
	assert.Contains(t, fullLogDetails, "scenario=full")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Equal(t, "run_tests", snapshotJobID)

	sess.LastTick = &session.LastTickState{Pipeline: instanceID, Job: "run_tests"}
	minimalOutput, minimalLogDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(context.Background(), scope.NewProjectScope(projectRoot), "ses-state-change", sess, activePipelines, scanWarnings)
	expectedMinimalOutput, err := FormatMinimalSummary("release", "run_tests", "1/1")
	require.NoError(t, err)
	assert.Equal(t, expectedMinimalOutput, minimalOutput)
	assert.Contains(t, minimalLogDetails, "scenario=minimal")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Equal(t, "run_tests", snapshotJobID)
}

func TestRenderTickJobPrompt(t *testing.T) {
	prepareMessage := "Preparation complete"
	prepareEndedAt := "20240115T103000Z"
	prepareStartedAt := "20240115T102900Z"

	wf := &workflow.Workflow{
		ID: "release",
		Jobs: []workflow.Job{
			{ID: "prepare", Prompt: "Prepare workspace"},
			{ID: "deploy", Prompt: "Previous={{.pre_job.message}} | Output={{.jobs.prepare.message}} | Current={{.job.id}}", Skill: "deploy-skill"},
		},
	}
	p := &pipeline.Pipeline{Jobs: map[string]*pipeline.JobData{
		"prepare": &pipeline.JobData{
			StartedAt: prepareStartedAt,
			EndedAt:   &prepareEndedAt,
			Message:   &prepareMessage,
		},
	}}

	tests := []struct {
		name       string
		pipeline   *pipeline.Pipeline
		workflow   *workflow.Workflow
		jobIndex   int
		wantPrompt string
		wantSkill  string
	}{
		{
			name:       "nil workflow returns empty values",
			pipeline:   p,
			workflow:   nil,
			jobIndex:   0,
			wantPrompt: "",
			wantSkill:  "",
		},
		{
			name:       "negative index returns empty values",
			pipeline:   p,
			workflow:   wf,
			jobIndex:   -1,
			wantPrompt: "",
			wantSkill:  "",
		},
		{
			name:       "out of bounds index returns empty values",
			pipeline:   p,
			workflow:   wf,
			jobIndex:   len(wf.Jobs),
			wantPrompt: "",
			wantSkill:  "",
		},
		{
			name:       "prompt templates render pipeline data",
			pipeline:   p,
			workflow:   wf,
			jobIndex:   1,
			wantPrompt: "Previous=Preparation complete | Output=Preparation complete | Current=deploy",
			wantSkill:  "deploy-skill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrompt, gotSkill := renderTickJobPrompt(context.Background(), tt.pipeline, tt.workflow, tt.jobIndex)
			assert.Equal(t, tt.wantPrompt, gotPrompt)
			assert.Equal(t, tt.wantSkill, gotSkill)
		})
	}
}

func TestRunTickInvariants(t *testing.T) {
	tests := []struct {
		name      string
		firstTick bool
		wantRan   int
	}{
		{
			name:      "first tick returns first failing invariant",
			firstTick: true,
			wantRan:   2,
		},
		{
			name:      "later ticks still return always invariant",
			firstTick: false,
			wantRan:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			writeTickInvariantFixture(t, projectRoot, "pass-always", `version: v0.1.0
id: pass-always
order: 10
auto: always
check:
  - shell: ":"
prompt: "Passing invariant should not fail"
`)
			writeTickInvariantFixture(t, projectRoot, "fail-always", `version: v0.1.0
id: fail-always
order: 20
description: Always failing invariant
auto: always
check:
  - shell: "exit 1"
    description: "always fails"
prompt: "Fix the always invariant"
`)
			writeTickInvariantFixture(t, projectRoot, "fail-session-start", `version: v0.1.0
id: fail-session-start
order: 30
auto: session_start
check:
  - shell: "exit 1"
prompt: "Fix the session-start invariant"
`)
			writeTickInvariantFixture(t, projectRoot, "fail-never", `version: v0.1.0
id: fail-never
order: 40
auto: never
check:
  - shell: "exit 1"
prompt: "Never auto-run this invariant"
`)
			writeTickInvariantFixture(t, projectRoot, "broken", `version: v0.1.0
id: broken
check:
  - shell: [not valid yaml
`)

			catalog, err := scope.NewProjectScope(projectRoot).Artifacts().Invariants().Catalog(true)
			require.NoError(t, err)
			result := runTickInvariants(context.Background(), catalog, projectRoot, tt.firstTick)
			require.NotNil(t, result.Failure)
			require.NotNil(t, result.Failure.Invariant)
			require.NotNil(t, result.Failure.FailedStep)
			assert.Equal(t, "fail-always", result.Failure.Invariant.ID)
			assert.Equal(t, "Always failing invariant", result.Failure.Invariant.Description)
			assert.Equal(t, "Fix the always invariant", result.Failure.Invariant.Prompt)
			assert.Equal(t, invariant.CheckStep{
				Shell:       "exit 1",
				Description: "always fails",
			}, result.Failure.FailedStep.Check)
			assert.Equal(t, "fail", result.Failure.FailedStep.Status)
			require.NotNil(t, result.Failure.FailedStep.ExitCode)
			assert.Equal(t, 1, *result.Failure.FailedStep.ExitCode)
			assert.Equal(t, "exit", result.Failure.FailedStep.FailureKind)
			assert.Empty(t, result.Failure.FailedStep.Output)
			assert.Equal(t, tt.wantRan, result.RanChecks)
			assert.Positive(t, result.TotalTime)
		})
	}
}

func TestRunTickInvariants_UsesProvidedContext(t *testing.T) {
	projectRoot := t.TempDir()
	writeTickInvariantFixture(t, projectRoot, "fail-always", `version: v0.1.0
id: fail-always
order: 10
auto: always
check:
  - shell: "exit 1"
prompt: "Fix the always invariant"
`)

	catalog, warning := loadTickInvariantCatalog(scope.NewProjectScope(projectRoot))
	require.Empty(t, warning)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := runTickInvariants(ctx, catalog, projectRoot, true)
	assert.Equal(t, 1, result.RanChecks)
	assert.Nil(t, result.Failure)
}

func writeTickWorkflowFixture(t *testing.T, projectRoot, workflowID, yamlContent string) {
	t.Helper()
	workflowsDir := filepath.Join(projectRoot, ".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, workflowID+".yaml"), []byte(yamlContent), 0o600))
}

func writeTickPipelineFixture(t *testing.T, projectRoot, instanceID, yamlContent string) {
	t.Helper()
	pipelinesDir := filepath.Join(projectRoot, ".argus", "pipelines")
	require.NoError(t, os.MkdirAll(pipelinesDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(pipelinesDir, instanceID+".yaml"), []byte(yamlContent), 0o600))
}

func writeTickInvariantFixture(t *testing.T, projectRoot, invariantID, yamlContent string) {
	t.Helper()
	invariantsDir := filepath.Join(projectRoot, ".argus", "invariants")
	require.NoError(t, os.MkdirAll(invariantsDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(invariantsDir, invariantID+".yaml"), []byte(yamlContent), 0o600))
}

func loadTickActivePipelines(t *testing.T, projectRoot string) ([]pipeline.ActivePipeline, []pipeline.ScanWarning) {
	t.Helper()
	activePipelines, scanWarnings, err := pipeline.ScanActivePipelines(filepath.Join(projectRoot, ".argus", "pipelines"))
	require.NoError(t, err)
	return activePipelines, scanWarnings
}

func captureDebugLogs(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	oldDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(oldDefault)

	fn()
	return buf.String()
}
