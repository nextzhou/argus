package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeJobDoneCmd runs the job-done command and captures stdout output.
func executeJobDoneCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newJobDoneCmd(), args...)
}

func writePipelineFixture(t *testing.T, instanceID, yamlContent string) {
	t.Helper()
	pipelinesDir := filepath.Join(".argus", "pipelines")
	require.NoError(t, os.MkdirAll(pipelinesDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(pipelinesDir, instanceID+".yaml"),
		[]byte(yamlContent), 0o600,
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
				assert.Contains(t, msg, "No active pipeline")
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
				require.Error(t, cmdErr)
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

func TestJobDoneDefaultTextNextJob(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtRunTests)

	stdout, stderr, err := executeTextCommand(t, newJobDoneCmd())
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "Argus: Job run_tests completed (2/5)")
	assert.Contains(t, stdout, "Next job: build")
	assert.Contains(t, stdout, "Prompt: Build the project")
	assert.NotContains(t, stdout, "Skill:")
	assert.Contains(t, stdout, `argus job-done --message "execution summary"`)
}

func TestJobDoneDefaultTextNextJobWithSkill(t *testing.T) {
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

	stdout, stderr, err := executeTextCommand(t, newJobDoneCmd())
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "Next job: step2")
	assert.Contains(t, stdout, "Skill: argus-deploy")
}

func TestJobDoneDefaultTextCompleted(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtVerify)

	stdout, stderr, err := executeTextCommand(t, newJobDoneCmd())
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "Argus: Job verify completed (5/5)")
	assert.Contains(t, stdout, "Pipeline release-20240101T000000Z is complete.")
}

func TestJobDoneDefaultTextFailed(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtRunTests)

	stdout, stderr, err := executeTextCommand(t, newJobDoneCmd(), "--fail")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "Argus: Job run_tests marked as failed. Pipeline stopped (2/5).")
	assert.Contains(t, stdout, "- Restart: argus workflow start release")
	assert.Contains(t, stdout, "- Cancel: argus workflow cancel")
}

func TestJobDoneDefaultTextFailedEarlyExit(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtRunTests)

	stdout, stderr, err := executeTextCommand(t, newJobDoneCmd(), "--fail", "--end-pipeline")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "Argus: Job run_tests marked as failed. Pipeline ended early (2/5).")
	assert.Contains(t, stdout, "- Restart: argus workflow start release")
}

func TestJobDoneDefaultTextEarlyExit(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)
	writePipelineFixture(t, testInstanceID, pipelineAtRunTests)

	stdout, stderr, err := executeTextCommand(t, newJobDoneCmd(), "--end-pipeline")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "Argus: Job run_tests completed. Pipeline ended early (2/5).")
}

func TestJobDoneDefaultTextNoPipeline(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "release", fiveJobWorkflow)

	stdout, stderr, err := executeTextCommand(t, newJobDoneCmd())
	require.Error(t, err)
	assert.Empty(t, stdout)

	assert.Contains(t, stderr, "Argus: No active pipeline.")
	assert.Contains(t, stderr, "Start one with argus workflow start <workflow-id>.")
}
