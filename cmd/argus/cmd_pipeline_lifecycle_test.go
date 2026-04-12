package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/pipeline"
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

	loadOnlyPipeline := func(t *testing.T) *pipeline.Pipeline {
		t.Helper()

		entries, err := os.ReadDir(filepath.Join(".argus", "pipelines"))
		require.NoError(t, err)
		require.Len(t, entries, 1)

		instanceID := entries[0].Name()[:len(entries[0].Name())-len(".yaml")]
		loaded, err := pipeline.LoadPipeline(filepath.Join(".argus", "pipelines"), instanceID)
		require.NoError(t, err)
		return loaded
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
		nextJob := mustJSONObject(t, data["next_job"])
		assert.Equal(t, "step_1", nextJob["id"])

		output, cmdErr = executeJobDoneCmd(t)
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "running", data["pipeline_status"])
		assert.Equal(t, "1/3", data["progress"])
		nextJob = mustJSONObject(t, data["next_job"])
		assert.Equal(t, "step_2", nextJob["id"])

		output, cmdErr = executeJobDoneCmd(t)
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "running", data["pipeline_status"])
		assert.Equal(t, "2/3", data["progress"])
		nextJob = mustJSONObject(t, data["next_job"])
		assert.Equal(t, "step_3", nextJob["id"])

		output, cmdErr = executeJobDoneCmd(t)
		require.NoError(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "ok", data["status"])
		assert.Equal(t, "completed", data["pipeline_status"])
		assert.Equal(t, "3/3", data["progress"])
		assert.Nil(t, data["next_job"])
		loaded := loadOnlyPipeline(t)
		assert.Equal(t, pipeline.StatusCompleted, loaded.Status)
		assert.Nil(t, loaded.CurrentJob)
		require.NotNil(t, loaded.EndedAt)

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
		cancelled := mustJSONArray(t, data["cancelled"])
		require.Len(t, cancelled, 1)
		loaded := loadOnlyPipeline(t)
		assert.Equal(t, pipeline.StatusCancelled, loaded.Status)
		require.NotNil(t, loaded.EndedAt)

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
		loaded := loadOnlyPipeline(t)
		assert.Equal(t, pipeline.StatusFailed, loaded.Status)
		require.NotNil(t, loaded.EndedAt)
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
		loaded := loadOnlyPipeline(t)
		assert.Equal(t, pipeline.StatusCompleted, loaded.Status)
		require.NotNil(t, loaded.EndedAt)
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
		require.Error(t, cmdErr)
		data = parseOutput(t, output)
		assert.Equal(t, "error", data["status"])
	})

	t.Run("job-done without active pipeline", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writeWorkflowFixture(t, "lifecycle-test", lifecycleWorkflow)

		output, cmdErr := executeJobDoneCmd(t)
		require.Error(t, cmdErr)
		data := parseOutput(t, output)
		assert.Equal(t, "error", data["status"])
	})
}
