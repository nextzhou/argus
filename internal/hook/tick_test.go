package hook

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/session"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
			want:    "[Argus] Warning: watch out\n",
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
			want:    "base text\n[Argus] Warning: watch out\n",
		},
		{
			name:    "base with trailing newline",
			base:    "base text\n",
			warning: "watch out",
			want:    "base text\n[Argus] Warning: watch out\n",
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

func TestLoadWorkflowForTick(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, projectRoot string) string
		assertFn func(t *testing.T, wf *workflow.Workflow, err error)
	}{
		{
			name: "successfully loads and resolves refs",
			setup: func(t *testing.T, projectRoot string) string {
				t.Helper()
				writeTickSharedFixture(t, projectRoot, `jobs:
  lint:
    prompt: "Run lint checks"
    skill: lint-skill
`)
				writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
jobs:
  - ref: lint
`)
				return filepath.Join(projectRoot, ".argus", "workflows", "release.yaml")
			},
			assertFn: func(t *testing.T, wf *workflow.Workflow, err error) {
				require.NoError(t, err)
				require.NotNil(t, wf)
				require.Len(t, wf.Jobs, 1)
				assert.Equal(t, workflow.Job{ID: "lint", Ref: "lint", Prompt: "Run lint checks", Skill: "lint-skill"}, wf.Jobs[0])
			},
		},
		{
			name: "parse error is returned",
			setup: func(t *testing.T, projectRoot string) string {
				t.Helper()
				path := filepath.Join(projectRoot, ".argus", "workflows", "broken.yaml")
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte("{{invalid yaml"), 0o644))
				return path
			},
			assertFn: func(t *testing.T, wf *workflow.Workflow, err error) {
				require.Error(t, err)
				assert.Nil(t, wf)
				assert.Contains(t, err.Error(), "parsing workflow file")
			},
		},
		{
			name: "ref resolution errors are returned",
			setup: func(t *testing.T, projectRoot string) string {
				t.Helper()
				writeTickSharedFixture(t, projectRoot, `jobs:
  lint:
    prompt: "Run lint checks"
`)
				writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
jobs:
  - ref: unknown_ref
`)
				return filepath.Join(projectRoot, ".argus", "workflows", "release.yaml")
			},
			assertFn: func(t *testing.T, wf *workflow.Workflow, err error) {
				require.Error(t, err)
				assert.Nil(t, wf)
				assert.Contains(t, err.Error(), "resolving ref")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			workflowPath := tt.setup(t, projectRoot)
			wf, err := loadWorkflowForTick(filepath.Join(projectRoot, ".argus", "workflows"), workflowPath)
			tt.assertFn(t, wf, err)
		})
	}
}

func TestResolveTickWorkflowRefs(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, projectRoot string) (string, *workflow.Workflow)
		assertFn func(t *testing.T, original *workflow.Workflow, resolved *workflow.Workflow, err error)
	}{
		{
			name: "workflow without refs is returned as is",
			setup: func(t *testing.T, projectRoot string) (string, *workflow.Workflow) {
				t.Helper()
				wf := &workflow.Workflow{ID: "release", Jobs: []workflow.Job{{ID: "run_tests", Prompt: "Run tests"}}}
				return filepath.Join(projectRoot, ".argus", "workflows", "release.yaml"), wf
			},
			assertFn: func(t *testing.T, original *workflow.Workflow, resolved *workflow.Workflow, err error) {
				require.NoError(t, err)
				assert.Same(t, original, resolved)
			},
		},
		{
			name: "workflow refs are resolved from shared definitions",
			setup: func(t *testing.T, projectRoot string) (string, *workflow.Workflow) {
				t.Helper()
				writeTickSharedFixture(t, projectRoot, `jobs:
  lint:
    prompt: "Run lint checks"
    skill: lint-skill
`)
				writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
jobs:
  - ref: lint
`)
				workflowPath := filepath.Join(projectRoot, ".argus", "workflows", "release.yaml")
				wf, err := workflow.ParseWorkflowFile(workflowPath)
				require.NoError(t, err)
				return workflowPath, wf
			},
			assertFn: func(t *testing.T, original *workflow.Workflow, resolved *workflow.Workflow, err error) {
				require.NoError(t, err)
				require.NotNil(t, resolved)
				require.Len(t, resolved.Jobs, 1)
				assert.Equal(t, workflow.Job{ID: "lint", Ref: "lint", Prompt: "Run lint checks", Skill: "lint-skill"}, resolved.Jobs[0])
				require.Len(t, original.Jobs, 1)
				assert.Empty(t, original.Jobs[0].Prompt)
			},
		},
		{
			name: "missing shared file returns error",
			setup: func(t *testing.T, projectRoot string) (string, *workflow.Workflow) {
				t.Helper()
				writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
jobs:
  - ref: lint
`)
				workflowPath := filepath.Join(projectRoot, ".argus", "workflows", "release.yaml")
				wf, err := workflow.ParseWorkflowFile(workflowPath)
				require.NoError(t, err)
				return workflowPath, wf
			},
			assertFn: func(t *testing.T, _ *workflow.Workflow, resolved *workflow.Workflow, err error) {
				require.Error(t, err)
				assert.Nil(t, resolved)
				assert.Contains(t, err.Error(), "loading shared definitions")
			},
		},
		{
			name: "invalid ref id returns error",
			setup: func(t *testing.T, projectRoot string) (string, *workflow.Workflow) {
				t.Helper()
				writeTickSharedFixture(t, projectRoot, `jobs:
  lint:
    prompt: "Run lint checks"
`)
				writeTickWorkflowFixture(t, projectRoot, "release", `version: v0.1.0
id: release
jobs:
  - ref: missing_ref
`)
				workflowPath := filepath.Join(projectRoot, ".argus", "workflows", "release.yaml")
				wf, err := workflow.ParseWorkflowFile(workflowPath)
				require.NoError(t, err)
				return workflowPath, wf
			},
			assertFn: func(t *testing.T, _ *workflow.Workflow, resolved *workflow.Workflow, err error) {
				require.Error(t, err)
				assert.Nil(t, resolved)
				assert.Contains(t, err.Error(), "resolving ref")
				assert.Contains(t, err.Error(), "missing_ref")
			},
		},
		{
			name: "nil workflow returns error",
			setup: func(t *testing.T, projectRoot string) (string, *workflow.Workflow) {
				t.Helper()
				return filepath.Join(projectRoot, ".argus", "workflows", "release.yaml"), nil
			},
			assertFn: func(t *testing.T, _ *workflow.Workflow, resolved *workflow.Workflow, err error) {
				require.Error(t, err)
				assert.Nil(t, resolved)
				assert.Contains(t, err.Error(), "workflow is nil")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			workflowPath, wf := tt.setup(t, projectRoot)
			resolved, err := resolveTickWorkflowRefs(filepath.Join(projectRoot, ".argus", "workflows"), workflowPath, wf)
			tt.assertFn(t, wf, resolved, err)
		})
	}
}

func TestFindTickJobNodes(t *testing.T) {
	tests := []struct {
		name      string
		buildDoc  func(t *testing.T) *yaml.Node
		wantCount int
		wantNil   bool
	}{
		{
			name: "valid document returns job nodes",
			buildDoc: func(t *testing.T) *yaml.Node {
				t.Helper()
				var doc yaml.Node
				require.NoError(t, yaml.Unmarshal([]byte(`version: v0.1.0
id: release
jobs:
  - id: lint
    prompt: "Run lint"
  - id: test
    prompt: "Run tests"
`), &doc))
				return &doc
			},
			wantCount: 2,
		},
		{
			name: "missing jobs key returns nil",
			buildDoc: func(t *testing.T) *yaml.Node {
				t.Helper()
				var doc yaml.Node
				require.NoError(t, yaml.Unmarshal([]byte(`version: v0.1.0
id: release
description: no jobs here
`), &doc))
				return &doc
			},
			wantNil: true,
		},
		{
			name: "malformed node returns nil",
			buildDoc: func(t *testing.T) *yaml.Node {
				t.Helper()
				return &yaml.Node{Kind: yaml.ScalarNode, Value: "not-a-document"}
			},
			wantNil: true,
		},
		{
			name: "empty jobs sequence returns empty result",
			buildDoc: func(t *testing.T) *yaml.Node {
				t.Helper()
				var doc yaml.Node
				require.NoError(t, yaml.Unmarshal([]byte(`version: v0.1.0
id: release
jobs: []
`), &doc))
				return &doc
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobNodes := findTickJobNodes(tt.buildDoc(t))
			if tt.wantNil {
				assert.Nil(t, jobNodes)
				return
			}
			if tt.wantCount == 0 {
				assert.Empty(t, jobNodes)
				return
			}
			assert.Len(t, jobNodes, tt.wantCount)
		})
	}
}

func TestRunTickInvariants(t *testing.T) {
	tests := []struct {
		name         string
		firstTick    bool
		wantIDs      []string
		wantByID     map[string]InvariantFailure
		wantFailures int
	}{
		{
			name:         "first tick runs always and session start invariants",
			firstTick:    true,
			wantIDs:      []string{"fail-always", "fail-session-start"},
			wantFailures: 2,
			wantByID: map[string]InvariantFailure{
				"fail-always": {
					ID:          "fail-always",
					Description: "Always failing invariant",
					Suggestion:  "Fix the always invariant",
				},
				"fail-session-start": {
					ID:          "fail-session-start",
					Description: "exit 1",
					Suggestion:  "Fix the session-start invariant",
				},
			},
		},
		{
			name:         "later ticks run only always invariants",
			firstTick:    false,
			wantIDs:      []string{"fail-always"},
			wantFailures: 1,
			wantByID: map[string]InvariantFailure{
				"fail-always": {
					ID:          "fail-always",
					Description: "Always failing invariant",
					Suggestion:  "Fix the always invariant",
				},
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

			failures := runTickInvariants(projectRoot, tt.firstTick)
			require.Len(t, failures, tt.wantFailures)

			gotIDs := make([]string, 0, len(failures))
			gotByID := make(map[string]InvariantFailure, len(failures))
			for _, failure := range failures {
				gotIDs = append(gotIDs, failure.ID)
				gotByID[failure.ID] = failure
			}

			assert.ElementsMatch(t, tt.wantIDs, gotIDs)
			for id, want := range tt.wantByID {
				assert.Equal(t, want, gotByID[id])
			}
			assert.NotContains(t, gotByID, "pass-always")
			assert.NotContains(t, gotByID, "fail-never")
			assert.NotContains(t, gotByID, "broken")
		})
	}
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

func writeTickInvariantFixture(t *testing.T, projectRoot, invariantID, yamlContent string) {
	t.Helper()
	invariantsDir := filepath.Join(projectRoot, ".argus", "invariants")
	require.NoError(t, os.MkdirAll(invariantsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(invariantsDir, invariantID+".yaml"), []byte(yamlContent), 0o644))
}

func writeTickSharedFixture(t *testing.T, projectRoot, yamlContent string) {
	t.Helper()
	workflowsDir := filepath.Join(projectRoot, ".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, "_shared.yaml"), []byte(yamlContent), 0o644))
}

func loadTickActivePipelines(t *testing.T, projectRoot string) ([]pipeline.ActivePipeline, []pipeline.ScanWarning) {
	t.Helper()
	activePipelines, scanWarnings, err := pipeline.ScanActivePipelines(filepath.Join(projectRoot, ".argus", "pipelines"))
	require.NoError(t, err)
	return activePipelines, scanWarnings
}
