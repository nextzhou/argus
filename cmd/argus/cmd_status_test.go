package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeStatusCmd runs the status command and captures stdout output.
// Tests using this helper must NOT call t.Parallel since os.Stdout is redirected.
func executeStatusCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := newStatusCmd()
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

func TestStatus(t *testing.T) {
	tests := []struct {
		name         string
		workflowYAML string
		workflowID   string
		pipelineYAML string
		instanceID   string
		wantErr      bool
		wantStatus   string
		checkJSON    func(t *testing.T, data map[string]any)
	}{
		{
			name:       "no active pipeline returns null pipeline",
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Nil(t, data["pipeline"])
				inv, ok := data["invariants"].(map[string]any)
				require.True(t, ok, "invariants should be an object")
				assert.Equal(t, float64(0), inv["passed"])
				assert.Equal(t, float64(0), inv["failed"])
				details, ok := inv["details"].([]any)
				require.True(t, ok, "details should be an array")
				assert.Empty(t, details)
				hints, ok := data["hints"].([]any)
				require.True(t, ok, "hints should be an array")
				assert.Empty(t, hints)
			},
		},
		{
			name:         "active pipeline with derived job statuses",
			workflowYAML: fiveJobWorkflow,
			workflowID:   "release",
			pipelineYAML: pipelineAtRunTests,
			instanceID:   testInstanceID,
			wantStatus:   "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				p, ok := data["pipeline"].(map[string]any)
				require.True(t, ok, "pipeline should be an object")
				assert.Equal(t, "release", p["workflow_id"])
				assert.Equal(t, "running", p["status"])
				assert.Equal(t, "run_tests", p["current_job"])
				assert.Equal(t, "20240101T000000Z", p["started_at"])
				assert.Nil(t, p["ended_at"])

				progress, ok := p["progress"].(map[string]any)
				require.True(t, ok, "progress should be an object")
				assert.Equal(t, float64(2), progress["current"])
				assert.Equal(t, float64(5), progress["total"])

				jobs, ok := p["jobs"].([]any)
				require.True(t, ok, "jobs should be an array")
				require.Len(t, jobs, 5)

				job0 := jobs[0].(map[string]any)
				assert.Equal(t, "lint", job0["id"])
				assert.Equal(t, "completed", job0["status"])
				assert.Nil(t, job0["message"])

				job1 := jobs[1].(map[string]any)
				assert.Equal(t, "run_tests", job1["id"])
				assert.Equal(t, "in_progress", job1["status"])
				assert.Nil(t, job1["message"])

				job2 := jobs[2].(map[string]any)
				assert.Equal(t, "build", job2["id"])
				assert.Equal(t, "pending", job2["status"])
				assert.Nil(t, job2["message"])

				job3 := jobs[3].(map[string]any)
				assert.Equal(t, "deploy", job3["id"])
				assert.Equal(t, "pending", job3["status"])

				job4 := jobs[4].(map[string]any)
				assert.Equal(t, "verify", job4["id"])
				assert.Equal(t, "pending", job4["status"])

				inv := data["invariants"].(map[string]any)
				assert.Equal(t, float64(0), inv["passed"])
				assert.Equal(t, float64(0), inv["failed"])

				hints := data["hints"].([]any)
				assert.Empty(t, hints)
			},
		},
		{
			name:         "active pipeline with message on completed job",
			workflowYAML: fiveJobWorkflow,
			workflowID:   "release",
			pipelineYAML: pipelineAtRunTestsWithMessage,
			instanceID:   testInstanceID,
			wantStatus:   "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				p := data["pipeline"].(map[string]any)
				jobs := p["jobs"].([]any)
				job0 := jobs[0].(map[string]any)
				assert.Equal(t, "lint passed cleanly", job0["message"])
			},
		},
		{
			name:         "multiple active pipelines returns error",
			workflowID:   "release",
			workflowYAML: fiveJobWorkflow,
			pipelineYAML: "",
			instanceID:   "",
			wantErr:      true,
			wantStatus:   "error",
			checkJSON: func(t *testing.T, data map[string]any) {
				msg, ok := data["message"].(string)
				require.True(t, ok)
				assert.Contains(t, msg, "多个活跃的 Pipeline")
			},
		},
		{
			name:         "workflow modification best-effort handling",
			workflowYAML: modifiedWorkflow,
			workflowID:   "release",
			pipelineYAML: pipelineWithMissingJob,
			instanceID:   testInstanceID,
			wantStatus:   "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				p := data["pipeline"].(map[string]any)
				assert.Equal(t, "release", p["workflow_id"])
				assert.Equal(t, "running", p["status"])
				assert.Nil(t, p["current_job"])

				hints := data["hints"].([]any)
				require.Len(t, hints, 1)
				assert.Contains(t, hints[0].(string), "workflow 定义已变更")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if tt.workflowYAML != "" {
				writeWorkflowFixture(t, tt.workflowID, tt.workflowYAML)
			}

			if tt.name == "multiple active pipelines returns error" {
				writeWorkflowFixture(t, "release", fiveJobWorkflow)
				writePipelineFixture(t, "release-20240101T000000Z", pipelineAtRunTests)
				writePipelineFixture(t, "release-20240102T000000Z", pipelineAtRunTests)
			} else if tt.pipelineYAML != "" {
				writePipelineFixture(t, tt.instanceID, tt.pipelineYAML)
			}

			output, cmdErr := executeStatusCmd(t)

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

const pipelineAtRunTestsWithMessage = `version: v0.1.0
workflow_id: release
status: running
current_job: run_tests
started_at: "20240101T000000Z"
jobs:
  lint:
    started_at: "20240101T000000Z"
    ended_at: "20240101T000100Z"
    message: "lint passed cleanly"
  run_tests:
    started_at: "20240101T000100Z"
`

const modifiedWorkflow = `version: v0.1.0
id: release
description: Modified workflow
jobs:
  - id: new_step1
    prompt: "New first step"
  - id: new_step2
    prompt: "New second step"
`

const pipelineWithMissingJob = `version: v0.1.0
workflow_id: release
status: running
current_job: old_job
started_at: "20240101T000000Z"
jobs:
  old_job:
    started_at: "20240101T000100Z"
`

func TestStatusMarkdownActivePipeline(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtRunTests)

	cmd := newStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[Argus] 项目状态")
	assert.Contains(t, output, "Pipeline: "+testInstanceID+" (running)")
	assert.Contains(t, output, "Workflow: release")
	assert.Contains(t, output, "进度 2/5")
	assert.Contains(t, output, "[done] lint")
	assert.Contains(t, output, "[>>]   run_tests")
	assert.Contains(t, output, "[ ]    build")
	assert.Contains(t, output, "[ ]    deploy")
	assert.Contains(t, output, "[ ]    verify")
	assert.Contains(t, output, "Invariant: 0 passed, 0 failed")
}

func TestStatusMarkdownNoPipeline(t *testing.T) {
	t.Chdir(t.TempDir())

	cmd := newStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[Argus] 项目状态")
	assert.Contains(t, output, "Pipeline: 无活跃 Pipeline")
	assert.Contains(t, output, "Invariant: 0 passed, 0 failed")
}

func TestStatusMarkdownWithMessage(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtRunTestsWithMessage)

	cmd := newStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[done] lint - lint passed cleanly")
}
