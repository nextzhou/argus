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

// executeJobDoneCmd runs the job-done command and captures stdout output.
// Tests using this helper must NOT call t.Parallel since os.Stdout is redirected.
func executeJobDoneCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := newJobDoneCmd()
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

func writePipelineFixture(t *testing.T, instanceID, yamlContent string) {
	t.Helper()
	pipelinesDir := filepath.Join(".argus", "pipelines")
	require.NoError(t, os.MkdirAll(pipelinesDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pipelinesDir, instanceID+".yaml"),
		[]byte(yamlContent), 0o644,
	))
}

const fiveJobWorkflow = `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: lint
    prompt: "Run lint checks"
  - id: run_tests
    prompt: "Run test suite"
  - id: build
    prompt: "Build the project"
  - id: deploy
    prompt: "Deploy to staging"
  - id: verify
    prompt: "Verify deployment"
`

const pipelineAtRunTests = `version: v0.1.0
workflow_id: release
status: running
current_job: run_tests
started_at: "20240101T000000Z"
jobs:
  lint:
    started_at: "20240101T000000Z"
    ended_at: "20240101T000100Z"
  run_tests:
    started_at: "20240101T000100Z"
`

const pipelineAtVerify = `version: v0.1.0
workflow_id: release
status: running
current_job: verify
started_at: "20240101T000000Z"
jobs:
  lint:
    started_at: "20240101T000000Z"
    ended_at: "20240101T000100Z"
  run_tests:
    started_at: "20240101T000100Z"
    ended_at: "20240101T000200Z"
  build:
    started_at: "20240101T000200Z"
    ended_at: "20240101T000300Z"
  deploy:
    started_at: "20240101T000300Z"
    ended_at: "20240101T000400Z"
  verify:
    started_at: "20240101T000400Z"
`

const testInstanceID = "release-20240101T000000Z"

func TestJobDone(t *testing.T) {
	tests := []struct {
		name         string
		pipelineYAML string
		args         []string
		wantErr      bool
		wantStatus   string
		checkJSON    func(t *testing.T, data map[string]any)
	}{
		{
			name:         "scenario 1: success with next job",
			pipelineYAML: pipelineAtRunTests,
			args:         []string{"--message", "tests passed"},
			wantStatus:   "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "running", data["pipeline_status"])
				assert.Equal(t, "2/5", data["progress"])
				nextJob, ok := data["next_job"].(map[string]any)
				require.True(t, ok, "next_job should be an object")
				assert.Equal(t, "build", nextJob["id"])
				assert.Contains(t, nextJob["prompt"].(string), "Build the project")
				assert.Nil(t, nextJob["skill"])
				assert.Nil(t, data["early_exit"])
				assert.Nil(t, data["failed_job"])
			},
		},
		{
			name:         "scenario 2: success last job pipeline complete",
			pipelineYAML: pipelineAtVerify,
			wantStatus:   "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "completed", data["pipeline_status"])
				assert.Equal(t, "5/5", data["progress"])
				assert.Nil(t, data["next_job"])
				assert.Nil(t, data["early_exit"])
				assert.Nil(t, data["failed_job"])
			},
		},
		{
			name:         "scenario 3: early exit with end-pipeline",
			pipelineYAML: pipelineAtRunTests,
			args:         []string{"--end-pipeline"},
			wantStatus:   "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "completed", data["pipeline_status"])
				assert.Equal(t, "2/5", data["progress"])
				assert.Nil(t, data["next_job"])
				assert.Equal(t, true, data["early_exit"])
				assert.Nil(t, data["failed_job"])
			},
		},
		{
			name:         "scenario 4: failure with fail flag",
			pipelineYAML: pipelineAtRunTests,
			args:         []string{"--fail"},
			wantStatus:   "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "failed", data["pipeline_status"])
				assert.Equal(t, "2/5", data["progress"])
				assert.Nil(t, data["next_job"])
				assert.Nil(t, data["early_exit"])
				assert.Equal(t, "run_tests", data["failed_job"])
			},
		},
		{
			name:       "scenario 5: no active pipeline",
			wantErr:    true,
			wantStatus: "error",
			checkJSON: func(t *testing.T, data map[string]any) {
				msg, ok := data["message"].(string)
				require.True(t, ok)
				assert.Contains(t, msg, "当前没有活跃的 Pipeline")
			},
		},
		{
			name:         "scenario 6: fail with end-pipeline",
			pipelineYAML: pipelineAtRunTests,
			args:         []string{"--fail", "--end-pipeline"},
			wantStatus:   "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "failed", data["pipeline_status"])
				assert.Equal(t, "2/5", data["progress"])
				assert.Nil(t, data["next_job"])
				assert.Equal(t, true, data["early_exit"])
				assert.Equal(t, "run_tests", data["failed_job"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeWorkflowFixture(t, "release", fiveJobWorkflow)

			if tt.pipelineYAML != "" {
				writePipelineFixture(t, testInstanceID, tt.pipelineYAML)
			}

			output, cmdErr := executeJobDoneCmd(t, tt.args...)

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

func TestJobDoneMarkdownNextJob(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtRunTests)

	cmd := newJobDoneCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[Argus] Job run_tests 完成 (2/5)")
	assert.Contains(t, output, "下一个 Job: build")
	assert.Contains(t, output, "Prompt: Build the project")
	assert.NotContains(t, output, "Skill:")
	assert.Contains(t, output, `argus job-done --message "执行结果摘要"`)
}

func TestJobDoneMarkdownNextJobWithSkill(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "skill-wf", `version: v0.1.0
id: skill-wf
jobs:
  - id: step1
    prompt: "Do step 1"
  - id: step2
    prompt: "Do step 2"
    skill: "argus-deploy"
`)
	writePipelineFixture(t, "skill-wf-20240101T000000Z", `version: v0.1.0
workflow_id: skill-wf
status: running
current_job: step1
started_at: "20240101T000000Z"
jobs:
  step1:
    started_at: "20240101T000000Z"
`)

	cmd := newJobDoneCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "下一个 Job: step2")
	assert.Contains(t, output, "Skill: argus-deploy")
}

func TestJobDoneMarkdownCompleted(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtVerify)

	cmd := newJobDoneCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[Argus] Job verify 完成 (5/5)")
	assert.Contains(t, output, "Pipeline release-20240101T000000Z 已全部完成。")
}

func TestJobDoneMarkdownFailed(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtRunTests)

	cmd := newJobDoneCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--fail", "--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[Argus] Job run_tests 标记为失败，Pipeline 已停止 (2/5)。")
	assert.Contains(t, output, "- 重新开始：argus workflow start release")
	assert.Contains(t, output, "- 取消：argus workflow cancel")
}

func TestJobDoneMarkdownFailedEarlyExit(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtRunTests)

	cmd := newJobDoneCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--fail", "--end-pipeline", "--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[Argus] Job run_tests 标记为失败，Pipeline 提前结束 (2/5)。")
	assert.Contains(t, output, "- 重新开始：argus workflow start release")
}

func TestJobDoneMarkdownEarlyExit(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtRunTests)

	cmd := newJobDoneCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--end-pipeline", "--markdown"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[Argus] Job run_tests 完成，Pipeline 提前结束 (2/5)。")
}

func TestJobDoneMarkdownNoPipeline(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)

	cmd := newJobDoneCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--markdown"})

	err := cmd.Execute()
	assert.Error(t, err)

	output := buf.String()
	assert.Contains(t, output, "[Argus] 当前没有活跃的 Pipeline。")
	assert.Contains(t, output, "可以使用 argus workflow start <workflow-id> 启动一个 workflow。")
}
