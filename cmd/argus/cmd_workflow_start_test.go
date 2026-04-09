package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// executeStartCmd runs the workflow start command and captures stdout output.
func executeStartCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newWorkflowStartCmd(), args...)
}

func writeWorkflowFixture(t *testing.T, id, yamlContent string) {
	t.Helper()
	workflowsDir := filepath.Join(".argus", "workflows")
	pipelinesDir := filepath.Join(".argus", "pipelines")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o755))
	require.NoError(t, os.MkdirAll(pipelinesDir, 0o755))
	if yamlContent != "" {
		require.NoError(t, os.WriteFile(
			filepath.Join(workflowsDir, id+".yaml"),
			[]byte(yamlContent), 0o644,
		))
	}
}

func TestWorkflowStart(t *testing.T) {
	tests := []struct {
		name         string
		workflowID   string
		workflowYAML string
		setupActive  bool
		wantErr      bool
		wantStatus   string
		checkJSON    func(t *testing.T, data map[string]any)
	}{
		{
			name:       "start workflow with skill",
			workflowID: "test-wf",
			workflowYAML: `version: v0.1.0
id: test-wf
description: Test workflow
jobs:
  - id: build
    prompt: "Build the project"
    skill: "argus-build"
`,
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "running", data["pipeline_status"])
				assert.Equal(t, "1/1", data["progress"])
				nextJob, ok := data["next_job"].(map[string]any)
				require.True(t, ok, "next_job should be an object")
				assert.Equal(t, "build", nextJob["id"])
				assert.Contains(t, nextJob["prompt"].(string), "Build the project")
				assert.Equal(t, "argus-build", nextJob["skill"])
			},
		},
		{
			name:       "skill is null when job has no skill",
			workflowID: "no-skill",
			workflowYAML: `version: v0.1.0
id: no-skill
jobs:
  - id: review
    prompt: "Review the code"
`,
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				nextJob, ok := data["next_job"].(map[string]any)
				require.True(t, ok)
				assert.Nil(t, nextJob["skill"], "skill should be null when not set")
			},
		},
		{
			name:       "progress shows correct total for multiple jobs",
			workflowID: "multi-job",
			workflowYAML: `version: v0.1.0
id: multi-job
jobs:
  - id: lint
    prompt: "Run lint"
  - id: test_code
    prompt: "Run tests"
  - id: build
    prompt: "Build"
`,
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "1/3", data["progress"])
				nextJob, ok := data["next_job"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "lint", nextJob["id"])
			},
		},
		{
			name:       "error when workflow file not found",
			workflowID: "nonexistent",
			wantErr:    true,
			wantStatus: "error",
		},
		{
			name:       "error when active pipeline exists",
			workflowID: "existing",
			workflowYAML: `version: v0.1.0
id: existing
jobs:
  - id: step1
    prompt: "Do step 1"
`,
			setupActive: true,
			wantErr:     true,
			wantStatus:  "error",
		},
		{
			name:       "error on invalid workflow id",
			workflowID: "../etc/passwd",
			wantErr:    true,
			wantStatus: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeWorkflowFixture(t, tt.workflowID, tt.workflowYAML)

			if tt.setupActive {
				pipelineYAML := `version: v0.1.0
workflow_id: existing
status: running
current_job: step1
started_at: "20240101T000000Z"
jobs:
  step1:
    started_at: "20240101T000000Z"
`
				path := filepath.Join(".argus", "pipelines", "existing-20240101T000000Z.yaml")
				require.NoError(t, os.WriteFile(path, []byte(pipelineYAML), 0o644))
			}

			output, cmdErr := executeStartCmd(t, tt.workflowID)

			if tt.wantErr {
				assert.Error(t, cmdErr)
			} else {
				require.NoError(t, cmdErr)
			}

			var data map[string]any
			require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
			assert.Equal(t, tt.wantStatus, data["status"])

			if tt.checkJSON != nil {
				tt.checkJSON(t, data)
			}
		})
	}
}

func TestWorkflowStartDefaultText(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "md-test", `version: v0.1.0
id: md-test
description: Default text test
jobs:
  - id: lint
    prompt: "Run linting"
    skill: "argus-lint"
  - id: build
    prompt: "Build project"
`)

	stdout, stderr, err := executeTextCommand(t, newWorkflowStartCmd(), "md-test")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "Argus: Pipeline")
	assert.Contains(t, stdout, "已启动 (1/2)")
	assert.Contains(t, stdout, "当前 Job: lint")
	assert.Contains(t, stdout, "Prompt: Run linting")
	assert.Contains(t, stdout, "Skill: argus-lint")
	assert.Contains(t, stdout, `argus job-done --message "执行结果摘要"`)
}

func TestWorkflowStartDefaultTextNoSkill(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "md-no-skill", `version: v0.1.0
id: md-no-skill
jobs:
  - id: review
    prompt: "Review code"
`)

	stdout, stderr, err := executeTextCommand(t, newWorkflowStartCmd(), "md-no-skill")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "当前 Job: review")
	assert.NotContains(t, stdout, "Skill:")
}

func writeSharedFixture(t *testing.T, yamlContent string) {
	t.Helper()
	workflowsDir := filepath.Join(".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(workflowsDir, "_shared.yaml"),
		[]byte(yamlContent), 0o644,
	))
}

func TestResolveRefs(t *testing.T) {
	tests := []struct {
		name         string
		workflowYAML string
		sharedYAML   string
		wantErr      bool
		wantErrMsg   string
		checkJobs    func(t *testing.T, jobs []workflow.Job)
	}{
		{
			name: "workflow with no refs returns unchanged",
			workflowYAML: `version: v0.1.0
id: no-refs
jobs:
  - id: build
    prompt: "Build the project"
  - id: test
    prompt: "Run tests"
`,
			wantErr: false,
			checkJobs: func(t *testing.T, jobs []workflow.Job) {
				require.Len(t, jobs, 2)
				assert.Equal(t, "build", jobs[0].ID)
				assert.Equal(t, "Build the project", jobs[0].Prompt)
				assert.Equal(t, "", jobs[0].Ref)
				assert.Equal(t, "test", jobs[1].ID)
				assert.Equal(t, "Run tests", jobs[1].Prompt)
				assert.Equal(t, "", jobs[1].Ref)
			},
		},
		{
			name: "workflow with valid ref resolves successfully",
			workflowYAML: `version: v0.1.0
id: with-refs
jobs:
  - ref: build_job
  - id: custom_test
    ref: test_job
    prompt: "Custom test prompt"
`,
			sharedYAML: `jobs:
  build_job:
    id: build_job
    prompt: "Build from shared"
    skill: "argus-build"
  test_job:
    id: test_job
    prompt: "Test from shared"
    skill: "argus-test"
`,
			wantErr: false,
			checkJobs: func(t *testing.T, jobs []workflow.Job) {
				require.Len(t, jobs, 2)
				assert.Equal(t, "build_job", jobs[0].ID)
				assert.Equal(t, "Build from shared", jobs[0].Prompt)
				assert.Equal(t, "argus-build", jobs[0].Skill)
				assert.Equal(t, "build_job", jobs[0].Ref)
				assert.Equal(t, "custom_test", jobs[1].ID)
				assert.Equal(t, "Custom test prompt", jobs[1].Prompt)
				assert.Equal(t, "argus-test", jobs[1].Skill)
				assert.Equal(t, "test_job", jobs[1].Ref)
			},
		},
		{
			name: "missing _shared.yaml returns error",
			workflowYAML: `version: v0.1.0
id: missing-shared
jobs:
  - ref: some_job
`,
			wantErr:    true,
			wantErrMsg: "loading shared definitions",
		},
		{
			name: "ref ID not found in _shared.yaml returns error",
			workflowYAML: `version: v0.1.0
id: ref-not-found
jobs:
  - ref: nonexistent_job
`,
			sharedYAML: `jobs:
  existing_job:
    id: existing_job
    prompt: "Exists"
`,
			wantErr:    true,
			wantErrMsg: "resolving ref for job",
		},
		{
			name: "mixed ref and non-ref jobs resolved correctly",
			workflowYAML: `version: v0.1.0
id: mixed-jobs
jobs:
  - id: direct_job
    prompt: "Direct prompt"
  - ref: shared_job
  - id: another_direct
    prompt: "Another direct"
`,
			sharedYAML: `jobs:
  shared_job:
    id: shared_job
    prompt: "Shared prompt"
    skill: "argus-shared"
`,
			wantErr: false,
			checkJobs: func(t *testing.T, jobs []workflow.Job) {
				require.Len(t, jobs, 3)
				assert.Equal(t, "direct_job", jobs[0].ID)
				assert.Equal(t, "Direct prompt", jobs[0].Prompt)
				assert.Equal(t, "", jobs[0].Ref)
				assert.Equal(t, "shared_job", jobs[1].ID)
				assert.Equal(t, "Shared prompt", jobs[1].Prompt)
				assert.Equal(t, "argus-shared", jobs[1].Skill)
				assert.Equal(t, "shared_job", jobs[1].Ref)
				assert.Equal(t, "another_direct", jobs[2].ID)
				assert.Equal(t, "Another direct", jobs[2].Prompt)
				assert.Equal(t, "", jobs[2].Ref)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeWorkflowFixture(t, "test-wf", tt.workflowYAML)
			if tt.sharedYAML != "" {
				writeSharedFixture(t, tt.sharedYAML)
			}

			var w workflow.Workflow
			data, err := os.ReadFile(filepath.Join(".argus", "workflows", "test-wf.yaml"))
			require.NoError(t, err)
			require.NoError(t, yaml.Unmarshal(data, &w))

			workflowsDir := filepath.Join(".argus", "workflows")
			workflowPath := filepath.Join(workflowsDir, "test-wf.yaml")
			err = resolveRefs(workflowsDir, workflowPath, &w)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
			} else {
				require.NoError(t, err)
				if tt.checkJobs != nil {
					tt.checkJobs(t, w.Jobs)
				}
			}
		})
	}
}

func TestFindJobNodes(t *testing.T) {
	tests := []struct {
		name      string
		yamlInput string
		wantCount int
		wantNil   bool
	}{
		{
			name: "valid YAML document with jobs sequence",
			yamlInput: `version: v0.1.0
id: test-wf
jobs:
  - id: job1
    prompt: "First job"
  - id: job2
    prompt: "Second job"
  - id: job3
    prompt: "Third job"
`,
			wantCount: 3,
			wantNil:   false,
		},
		{
			name: "missing jobs key returns nil",
			yamlInput: `version: v0.1.0
id: test-wf
description: "No jobs here"
`,
			wantCount: 0,
			wantNil:   true,
		},
		{
			name: "empty jobs sequence returns empty slice",
			yamlInput: `version: v0.1.0
id: test-wf
jobs: []
`,
			wantCount: 0,
			wantNil:   true,
		},
		{
			name: "single job in sequence",
			yamlInput: `version: v0.1.0
id: test-wf
jobs:
  - id: only_job
    prompt: "Only one"
`,
			wantCount: 1,
			wantNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc yaml.Node
			err := yaml.Unmarshal([]byte(tt.yamlInput), &doc)
			require.NoError(t, err)

			result := findJobNodes(&doc)

			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Len(t, result, tt.wantCount)
			}
		})
	}
}
