package hook

import (
	"bytes"
	"errors"
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
		"claude-code",
		false,
		bytes.NewBufferString(`{"session_id":"ses-no-pipeline","cwd":"`+projectRoot+`"}`),
		&out,
		projectRoot,
		sessionBaseDir,
	)
	require.NoError(t, err)

	output := out.String()
	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "Argus:")
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
	assertHookSafeTickText(t, out.String())
	assert.Contains(t, out.String(), "Argus warning")
	assert.Contains(t, out.String(), "not inside an Argus project")
}

func TestLoadTickWorkflowSummaries_LoadError(t *testing.T) {
	logs := captureDebugLogs(t, func() {
		summaries := loadTickWorkflowSummaries(errorLoadingScope{workflowErr: errors.New("boom")})
		assert.Nil(t, summaries)
	})

	assert.Contains(t, logs, "tick: could not load workflow summaries")
	assert.Contains(t, logs, "boom")
}

func TestRunTickInvariants_LoadError(t *testing.T) {
	logs := captureDebugLogs(t, func() {
		failure := runTickInvariants(errorLoadingScope{invariantErr: errors.New("boom")}, true)
		assert.Nil(t, failure)
	})

	assert.Contains(t, logs, "tick: could not load invariants")
	assert.Contains(t, logs, "boom")
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
				Workflow: "argus-project-init",
				Prompt:   "<<<ARGUS_INIT_REQUIRED>>>",
			},
			want: "Run argus workflow start argus-project-init",
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

func TestDescribeInvariant(t *testing.T) {
	tests := []struct {
		name string
		inv  *invariant.Invariant
		want string
	}{
		{
			name: "description takes priority",
			inv: &invariant.Invariant{
				Description: "Project is initialized",
				Check:       []invariant.CheckStep{{Shell: `test -d .argus`}},
			},
			want: "Project is initialized",
		},
		{
			name: "shell commands are joined when description is absent",
			inv: &invariant.Invariant{
				Check: []invariant.CheckStep{{Shell: `test -d .argus`}, {Shell: `test -f AGENTS.md`}},
			},
			want: "test -d .argus; test -f AGENTS.md",
		},
		{
			name: "nil invariant returns empty string",
			inv:  nil,
			want: "",
		},
		{
			name: "empty checks return empty string",
			inv:  &invariant.Invariant{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, describeInvariant(tt.inv))
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

	output, logDetails := buildNoActivePipelineOutput(scope.NewProjectScope(projectRoot), false)

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
description: Project not initialized
auto: always
check:
  - shell: "exit 1"
prompt: "Initialize the project first"
`)

	output, logDetails := buildNoActivePipelineOutput(scope.NewProjectScope(projectRoot), false)

	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "Argus: Invariant check failed:")
	assert.Contains(t, output, "argus-project-init")
	assert.Contains(t, output, "Initialize the project first")
	assert.NotContains(t, output, "No active pipeline")
	assert.NotContains(t, output, "release")
	assert.Contains(t, logDetails, "scenario=invariant-failed")
}

func TestBuildNoActivePipelineOutput_NoWorkflowsReturnsEmpty(t *testing.T) {
	output, logDetails := buildNoActivePipelineOutput(scope.NewProjectScope(t.TempDir()), false)

	assert.Empty(t, output)
	assert.Contains(t, logDetails, "active=0")
	assert.Contains(t, logDetails, "scenario=no-output")
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
	output, logDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(scope.NewProjectScope(projectRoot), "ses-snoozed", &session.Session{SnoozedPipelines: []string{instanceID}}, activePipelines, scanWarnings)

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
	output, logDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(scope.NewProjectScope(projectRoot), "ses-missing-job", &session.Session{}, activePipelines, scanWarnings)

	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "Argus: No active pipeline.")
	assert.Contains(t, output, "active pipeline is missing current job state")
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
	output, logDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(scope.NewProjectScope(projectRoot), "ses-missing-workflow", &session.Session{}, activePipelines, scanWarnings)

	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "Argus: No active pipeline.")
	assert.Contains(t, output, "could not load workflow missing")
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
	output, logDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(scope.NewProjectScope(projectRoot), "ses-workflow-mismatch", &session.Session{}, activePipelines, scanWarnings)

	assertHookSafeTickText(t, output)
	assert.Contains(t, output, "Argus: No active pipeline.")
	assert.Contains(t, output, "current job deploy was not found in workflow release")
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

	fullOutput, fullLogDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(scope.NewProjectScope(projectRoot), "ses-state-change", sess, activePipelines, scanWarnings)
	expectedFullOutput, err := FormatFullContext(instanceID, "release", "1/1", "run_tests", "Run tests with context", "test-skill", "ses-state-change")
	require.NoError(t, err)
	assert.Equal(t, expectedFullOutput, fullOutput)
	assert.Contains(t, fullLogDetails, "scenario=full")
	assert.Equal(t, instanceID, snapshotPipelineID)
	assert.Equal(t, "run_tests", snapshotJobID)

	sess.LastTick = &session.LastTickState{Pipeline: instanceID, Job: "run_tests"}
	minimalOutput, minimalLogDetails, snapshotPipelineID, snapshotJobID := buildActivePipelineOutput(scope.NewProjectScope(projectRoot), "ses-state-change", sess, activePipelines, scanWarnings)
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
			gotPrompt, gotSkill := renderTickJobPrompt(tt.pipeline, tt.workflow, tt.jobIndex)
			assert.Equal(t, tt.wantPrompt, gotPrompt)
			assert.Equal(t, tt.wantSkill, gotSkill)
		})
	}
}

func TestRunTickInvariants(t *testing.T) {
	tests := []struct {
		name        string
		firstTick   bool
		wantFailure *InvariantFailure
	}{
		{
			name:      "first tick returns first failing invariant",
			firstTick: true,
			wantFailure: &InvariantFailure{
				ID:          "fail-always",
				Description: "Always failing invariant",
				Suggestion:  "Fix the always invariant",
			},
		},
		{
			name:      "later ticks still return always invariant",
			firstTick: false,
			wantFailure: &InvariantFailure{
				ID:          "fail-always",
				Description: "Always failing invariant",
				Suggestion:  "Fix the always invariant",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			writeTickInvariantFixture(t, projectRoot, "pass-always", `version: v0.1.0
id: pass-always
auto: always
check:
  - shell: ":"
prompt: "Passing invariant should not fail"
`)
			writeTickInvariantFixture(t, projectRoot, "fail-always", `version: v0.1.0
id: fail-always
description: Always failing invariant
auto: always
check:
  - shell: "exit 1"
prompt: "Fix the always invariant"
`)
			writeTickInvariantFixture(t, projectRoot, "fail-session-start", `version: v0.1.0
id: fail-session-start
auto: session_start
check:
  - shell: "exit 1"
prompt: "Fix the session-start invariant"
`)
			writeTickInvariantFixture(t, projectRoot, "fail-never", `version: v0.1.0
id: fail-never
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

			failure := runTickInvariants(scope.NewProjectScope(projectRoot), tt.firstTick)
			require.NotNil(t, failure)
			assert.Equal(t, *tt.wantFailure, *failure)
		})
	}
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

type errorLoadingScope struct {
	workflowErr  error
	invariantErr error
}

func (s errorLoadingScope) LoadInvariants() ([]*invariant.Invariant, error) {
	return nil, s.invariantErr
}

func (s errorLoadingScope) ScanActivePipelines() ([]pipeline.ActivePipeline, []pipeline.ScanWarning, error) {
	return nil, nil, nil
}

func (s errorLoadingScope) LoadWorkflow(string) (*workflow.Workflow, error) {
	return nil, nil
}

func (s errorLoadingScope) LoadWorkflowSummaries() ([]scope.WorkflowSummary, error) {
	return nil, s.workflowErr
}

func (s errorLoadingScope) ProjectRoot() string {
	return ""
}

func (s errorLoadingScope) PipelinesDir() string {
	return ""
}

func (s errorLoadingScope) WorkflowsDir() string {
	return ""
}

func (s errorLoadingScope) LogsDir() string {
	return ""
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
