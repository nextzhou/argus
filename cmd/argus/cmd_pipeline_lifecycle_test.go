package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const lifecycleWorkflow = `version: v0.1.0
id: lifecycle-test
jobs:
  - id: step_1
    prompt: "First step"
  - id: step_2
    prompt: "Second step"
  - id: step_3
    prompt: "Third step"
`

func TestPipelineLifecycle(t *testing.T) {
	parseOutput := func(t *testing.T, output []byte) map[string]any {
		t.Helper()

		var data map[string]any
		require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
		return data
	}

	readOnlyPipelineFile := func(t *testing.T) string {
		t.Helper()

		entries, err := os.ReadDir(filepath.Join(".argus", "pipelines"))
		require.NoError(t, err)
		require.Len(t, entries, 1)

		file, err := os.Open(filepath.Join(".argus", "pipelines", entries[0].Name()))
		require.NoError(t, err)

		content, err := io.ReadAll(file)
		require.NoError(t, err)
		require.NoError(t, file.Close())

		return string(content)
	}

	t.Run("complete lifecycle", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writeWorkflowFixture(t, "lifecycle-test", lifecycleWorkflow)

		output, cmdErr := executeStartCmd(t, "lifecycle-test")
		require.NoError(t, cmdErr)
		data := parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "running", data["pipeline_status"])
		assert.Equal(t, "1/3", data["progress"])
		nextJob, ok := data["next_job"].(map[string]any)
		require.True(t, ok, "next_job should be an object")
		assert.Equal(t, "step_1", nextJob["id"])

		output, cmdErr = executeJobDoneCmd(t)
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "running", data["pipeline_status"])
		assert.Equal(t, "1/3", data["progress"])
		nextJob, ok = data["next_job"].(map[string]any)
		require.True(t, ok, "next_job should be an object")
		assert.Equal(t, "step_2", nextJob["id"])

		output, cmdErr = executeJobDoneCmd(t)
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "running", data["pipeline_status"])
		assert.Equal(t, "2/3", data["progress"])
		nextJob, ok = data["next_job"].(map[string]any)
		require.True(t, ok, "next_job should be an object")
		assert.Equal(t, "step_3", nextJob["id"])

		output, cmdErr = executeJobDoneCmd(t)
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "completed", data["pipeline_status"])
		assert.Equal(t, "3/3", data["progress"])
		assert.Nil(t, data["next_job"])
		assert.Contains(t, readOnlyPipelineFile(t), "status: completed")

		output, cmdErr = executeStatusCmd(t)
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Nil(t, data["pipeline"])
	})

	t.Run("cancel running pipeline", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writeWorkflowFixture(t, "lifecycle-test", lifecycleWorkflow)

		output, cmdErr := executeStartCmd(t, "lifecycle-test")
		require.NoError(t, cmdErr)
		data := parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "running", data["pipeline_status"])

		output, cmdErr = executeCancelCmd(t)
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		cancelled, ok := data["cancelled"].([]any)
		require.True(t, ok, "cancelled should be an array")
		require.Len(t, cancelled, 1)
		assert.Contains(t, readOnlyPipelineFile(t), "status: cancelled")

		output, cmdErr = executeStatusCmd(t)
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Nil(t, data["pipeline"])
	})

	t.Run("job failure stops pipeline", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writeWorkflowFixture(t, "lifecycle-test", lifecycleWorkflow)

		output, cmdErr := executeStartCmd(t, "lifecycle-test")
		require.NoError(t, cmdErr)
		data := parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "running", data["pipeline_status"])

		output, cmdErr = executeJobDoneCmd(t, "--fail")
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "failed", data["pipeline_status"])
		assert.Equal(t, "step_1", data["failed_job"])
		assert.Nil(t, data["next_job"])
		assert.Contains(t, readOnlyPipelineFile(t), "status: failed")
	})

	t.Run("early exit completes pipeline", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writeWorkflowFixture(t, "lifecycle-test", lifecycleWorkflow)

		output, cmdErr := executeStartCmd(t, "lifecycle-test")
		require.NoError(t, cmdErr)
		data := parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "running", data["pipeline_status"])

		output, cmdErr = executeJobDoneCmd(t, "--end-pipeline")
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "completed", data["pipeline_status"])
		assert.Equal(t, true, data["early_exit"])
		assert.Nil(t, data["next_job"])
		assert.Contains(t, readOnlyPipelineFile(t), "status: completed")
	})

	t.Run("duplicate start rejected", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writeWorkflowFixture(t, "lifecycle-test", lifecycleWorkflow)

		output, cmdErr := executeStartCmd(t, "lifecycle-test")
		require.NoError(t, cmdErr)
		data := parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "running", data["pipeline_status"])

		output, cmdErr = executeStartCmd(t, "lifecycle-test")
		assert.Error(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "error", data["status"])
	})

	t.Run("job-done without active pipeline", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writeWorkflowFixture(t, "lifecycle-test", lifecycleWorkflow)

		output, cmdErr := executeJobDoneCmd(t)
		assert.Error(t, cmdErr)
		data := parseOutput(t, output)
		assert.Equal(t, "error", data["status"])
	})
}
