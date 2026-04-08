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

func writeTickWorkflowFixture(t *testing.T, projectRoot, workflowID, yamlContent string) {
	t.Helper()
	workflowsDir := filepath.Join(projectRoot, ".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, workflowID+".yaml"), []byte(yamlContent), 0o644))
}
