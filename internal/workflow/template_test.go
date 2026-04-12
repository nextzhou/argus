package workflow

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildContextWithRuntime(t *testing.T) {
	workflowDef := &Workflow{
		ID:          "release",
		Description: "Release workflow",
		Jobs: []Job{
			{ID: "prepare"},
			{ID: "run_tests"},
			{ID: "deploy"},
		},
	}

	runtime := templateRuntime{
		env: func() map[string]string {
			return map[string]string{"ARGUS_TEMPLATE_ENV": "from-env"}
		},
		gitBranch: func(context.Context) string {
			return "feat/test-branch"
		},
		projectRoot: func() string {
			return "/tmp/project"
		},
	}

	tests := []struct {
		name              string
		jobIdx            int
		jobs              map[string]*PipelineJobData
		wantJobID         string
		wantJobIndex      int
		wantPreJobID      string
		wantPreJobMessage string
		wantCompletedJobs []string
	}{
		{
			name:              "first job uses empty previous job fields",
			jobIdx:            0,
			jobs:              map[string]*PipelineJobData{},
			wantJobID:         "prepare",
			wantJobIndex:      0,
			wantPreJobID:      "",
			wantPreJobMessage: "",
			wantCompletedJobs: []string{},
		},
		{
			name:   "later job includes previous message and completed jobs only",
			jobIdx: 2,
			jobs: map[string]*PipelineJobData{
				"prepare": {
					StartedAt: "20240115T103000Z",
					EndedAt:   new("20240115T103100Z"),
					Message:   new("prepared"),
				},
				"run_tests": {
					StartedAt: "20240115T103101Z",
					EndedAt:   new("20240115T103200Z"),
					Message:   new("tests passed"),
				},
				"deploy": {
					StartedAt: "20240115T103201Z",
					EndedAt:   nil,
					Message:   nil,
				},
				"notify": {
					StartedAt: "20240115T103201Z",
					EndedAt:   new("20240115T103250Z"),
					Message:   new(""),
				},
			},
			wantJobID:         "deploy",
			wantJobIndex:      2,
			wantPreJobID:      "run_tests",
			wantPreJobMessage: "tests passed",
			wantCompletedJobs: []string{"prepare", "run_tests"},
		},
		{
			name:              "invalid job index still returns runtime context",
			jobIdx:            99,
			jobs:              map[string]*PipelineJobData{},
			wantJobID:         "",
			wantJobIndex:      0,
			wantPreJobID:      "",
			wantPreJobMessage: "",
			wantCompletedJobs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := buildContextWithRuntime(context.Background(), tt.jobs, workflowDef, tt.jobIdx, runtime)
			require.NotNil(t, ctx)

			assert.Equal(t, workflowDef.ID, ctx.Workflow.ID)
			assert.Equal(t, workflowDef.Description, ctx.Workflow.Description)
			assert.Equal(t, tt.wantJobID, ctx.Job.ID)
			assert.Equal(t, tt.wantJobIndex, ctx.Job.Index)
			assert.Equal(t, tt.wantPreJobID, ctx.PreJob.ID)
			assert.Equal(t, tt.wantPreJobMessage, ctx.PreJob.Message)
			assert.Equal(t, "feat/test-branch", ctx.Git.Branch)
			assert.Equal(t, "/tmp/project", ctx.Project.Root)
			assert.Equal(t, "from-env", ctx.Env["ARGUS_TEMPLATE_ENV"])

			completedJobs := slices.Sorted(maps.Keys(ctx.Jobs))
			assert.ElementsMatch(t, tt.wantCompletedJobs, completedJobs)
		})
	}
}

func TestTemplateRenderPrompt(t *testing.T) {
	ctx := &TemplateContext{
		Workflow: TemplateWorkflowContext{
			ID:          "release",
			Description: "Release workflow",
		},
		Job: TemplateJobContext{
			ID:    "deploy",
			Index: 2,
		},
		PreJob: TemplatePreJobContext{
			ID:      "run_tests",
			Message: "tests passed",
		},
		Git: TemplateGitContext{
			Branch: "feat/m3-workflow-execution",
		},
		Project: TemplateProjectContext{
			Root: "/tmp/project",
		},
		Env: map[string]string{
			"ARGUS_TEMPLATE_ENV": "from-env",
		},
		Jobs: map[string]TemplateJobOutputContext{
			"run_tests": {Message: "tests passed"},
		},
	}

	tests := []struct {
		name                string
		ctx                 *TemplateContext
		prompt              string
		want                string
		wantWarningCount    int
		wantWarningContains []string
	}{
		{
			name:   "replaces known variables",
			ctx:    ctx,
			prompt: "workflow={{ .workflow.id }} desc={{ .workflow.description }} job={{ .job.id }} idx={{ .job.index }} pre={{ .pre_job.id }} msg={{ .pre_job.message }} branch={{ .git.branch }} root={{ .project.root }} env={{ .env.ARGUS_TEMPLATE_ENV }} tests={{ .jobs.run_tests.message }}",
			want:   "workflow=release desc=Release workflow job=deploy idx=2 pre=run_tests msg=tests passed branch=feat/m3-workflow-execution root=/tmp/project env=from-env tests=tests passed",
		},
		{
			name:             "preserves unknown placeholders and warns",
			ctx:              ctx,
			prompt:           "known={{ .workflow.id }} missing_field={{ .workflow.missing }} missing_env={{ .env.NOT_SET }} missing_job={{ .jobs.deploy.message }} missing_category={{ .future.value }}",
			want:             "known=release missing_field={{ .workflow.missing }} missing_env={{ .env.NOT_SET }} missing_job={{ .jobs.deploy.message }} missing_category={{ .future.value }}",
			wantWarningCount: 4,
			wantWarningContains: []string{
				"{{ .workflow.missing }}",
				"{{ .env.NOT_SET }}",
				"{{ .jobs.deploy.message }}",
				"{{ .future.value }}",
			},
		},
		{
			name: "first job previous fields resolve to empty strings",
			ctx: &TemplateContext{
				Workflow: TemplateWorkflowContext{ID: "release"},
				Job:      TemplateJobContext{ID: "prepare", Index: 0},
				Env:      map[string]string{},
				Jobs:     map[string]TemplateJobOutputContext{},
			},
			prompt: "pre={{ .pre_job.id }}|{{ .pre_job.message }}",
			want:   "pre=|",
		},
		{
			name:             "invalid template returns original prompt with warning",
			ctx:              ctx,
			prompt:           "{{ .workflow.id ",
			want:             "{{ .workflow.id ",
			wantWarningCount: 1,
			wantWarningContains: []string{
				"invalid template syntax",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warnings := RenderPrompt(tt.prompt, tt.ctx)

			assert.Equal(t, tt.want, got)
			assert.Len(t, warnings, tt.wantWarningCount)
			for _, want := range tt.wantWarningContains {
				assert.Condition(t, func() bool {
					for _, warning := range warnings {
						if strings.Contains(warning, want) {
							return true
						}
					}
					return false
				})
			}
		})
	}
}
