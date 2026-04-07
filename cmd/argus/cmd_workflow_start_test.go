package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeStartCmd runs the workflow start command and captures stdout output.
// Tests using this helper must NOT call t.Parallel since os.Stdout is redirected.
func executeStartCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := newWorkflowStartCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	cmdErr := cmd.Execute()

	require.NoError(t, w.Close())
	os.Stdout = old

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return out, cmdErr
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

func TestWorkflowStartMarkdown(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "md-test", `version: v0.1.0
id: md-test
description: Markdown test
jobs:
  - id: lint
    prompt: "Run linting"
    skill: "argus-lint"
  - id: build
    prompt: "Build project"
`)

	cmd := newWorkflowStartCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"md-test", "--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[Argus] Pipeline")
	assert.Contains(t, output, "已启动 (1/2)")
	assert.Contains(t, output, "当前 Job: lint")
	assert.Contains(t, output, "Prompt: Run linting")
	assert.Contains(t, output, "Skill: argus-lint")
	assert.Contains(t, output, `argus job-done --message "执行结果摘要"`)
}

func TestWorkflowStartMarkdownNoSkill(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "md-no-skill", `version: v0.1.0
id: md-no-skill
jobs:
  - id: review
    prompt: "Review code"
`)

	cmd := newWorkflowStartCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"md-no-skill", "--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "当前 Job: review")
	assert.NotContains(t, output, "Skill:")
}
