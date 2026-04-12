package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInstanceID(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		workflowID string
		t          time.Time
		want       string
	}{
		{
			name:       "basic workflow id",
			workflowID: "release",
			t:          ts,
			want:       "release-20240115T103000Z",
		},
		{
			name:       "hyphenated workflow id",
			workflowID: "code-review",
			t:          ts,
			want:       "code-review-20240115T103000Z",
		},
		{
			name:       "non-UTC time is converted",
			workflowID: "build",
			t:          time.Date(2024, 6, 1, 12, 0, 0, 0, time.FixedZone("EST", -5*3600)),
			want:       "build-20240601T170000Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewInstanceID(tt.workflowID, tt.t)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSavePipelineCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pipelines")

	p := &Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "release",
		Status:     "running",
		CurrentJob: new("lint"),
		StartedAt:  "20240115T103000Z",
		Jobs:       map[string]*JobData{},
	}

	err := SavePipeline(dir, "release-20240115T103000Z", p)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(filepath.Join(dir, "release-20240115T103000Z.yaml"))
	assert.NoError(t, err)
}

func TestSaveAndLoadPipelineRoundTrip(t *testing.T) {
	dir := t.TempDir()

	endedAt := "20240115T103100Z"
	msg := "All lint checks passed"

	original := &Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "release",
		Status:     "running",
		CurrentJob: new("run_tests"),
		StartedAt:  "20240115T103000Z",
		EndedAt:    nil,
		Jobs: map[string]*JobData{
			"lint": {
				StartedAt: "20240115T103005Z",
				EndedAt:   &endedAt,
				Message:   &msg,
			},
			"run_tests": {
				StartedAt: "20240115T103105Z",
				EndedAt:   nil,
				Message:   nil,
			},
		},
	}

	instanceID := "release-20240115T103000Z"

	err := SavePipeline(dir, instanceID, original)
	require.NoError(t, err)

	loaded, err := LoadPipeline(dir, instanceID)
	require.NoError(t, err)

	assert.Equal(t, original.Version, loaded.Version)
	assert.Equal(t, original.WorkflowID, loaded.WorkflowID)
	assert.Equal(t, original.Status, loaded.Status)
	assert.Equal(t, *original.CurrentJob, *loaded.CurrentJob)
	assert.Equal(t, original.StartedAt, loaded.StartedAt)
	assert.Nil(t, loaded.EndedAt)

	// Verify jobs
	require.Len(t, loaded.Jobs, 2)

	lint := loaded.Jobs["lint"]
	require.NotNil(t, lint)
	assert.Equal(t, "20240115T103005Z", lint.StartedAt)
	assert.Equal(t, &endedAt, lint.EndedAt)
	assert.Equal(t, &msg, lint.Message)

	runTests := loaded.Jobs["run_tests"]
	require.NotNil(t, runTests)
	assert.Equal(t, "20240115T103105Z", runTests.StartedAt)
	assert.Nil(t, runTests.EndedAt)
	assert.Nil(t, runTests.Message)
}

func TestSaveAndLoadPipelineCompletedState(t *testing.T) {
	dir := t.TempDir()

	endedAt := "20240115T104000Z"

	p := &Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "release",
		Status:     "completed",
		CurrentJob: nil,
		StartedAt:  "20240115T103000Z",
		EndedAt:    &endedAt,
		Jobs:       map[string]*JobData{},
	}

	instanceID := "release-20240115T103000Z"

	err := SavePipeline(dir, instanceID, p)
	require.NoError(t, err)

	loaded, err := LoadPipeline(dir, instanceID)
	require.NoError(t, err)

	assert.Equal(t, "completed", loaded.Status)
	assert.Nil(t, loaded.CurrentJob)
	assert.Equal(t, &endedAt, loaded.EndedAt)
}

func TestLoadPipelineNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadPipeline(dir, "nonexistent-20240115T103000Z")
	assert.Error(t, err)
}

func TestLoadPipelineInvalidYAML(t *testing.T) {
	dir := t.TempDir()

	// Write invalid YAML
	err := os.WriteFile(filepath.Join(dir, "bad-20240115T103000Z.yaml"), []byte("{{invalid yaml"), 0o600)
	require.NoError(t, err)

	_, err = LoadPipeline(dir, "bad-20240115T103000Z")
	assert.Error(t, err)
}

func TestLoadPipelineIncompatibleVersion(t *testing.T) {
	dir := t.TempDir()

	content := `version: v9.0.0
workflow_id: release
status: running
current_job: lint
started_at: "20240115T103000Z"
jobs: {}
`
	err := os.WriteFile(filepath.Join(dir, "release-20240115T103000Z.yaml"), []byte(content), 0o600)
	require.NoError(t, err)

	_, err = LoadPipeline(dir, "release-20240115T103000Z")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestLoadPipelineUnknownFields(t *testing.T) {
	dir := t.TempDir()

	content := `version: v0.1.0
workflow_id: release
status: running
current_job: lint
started_at: "20240115T103000Z"
unknown_field: should_fail
jobs: {}
`
	err := os.WriteFile(filepath.Join(dir, "release-20240115T103000Z.yaml"), []byte(content), 0o600)
	require.NoError(t, err)

	_, err = LoadPipeline(dir, "release-20240115T103000Z")
	assert.Error(t, err)
}

func TestScanActivePipelinesRunning(t *testing.T) {
	dir := t.TempDir()

	// Create a running pipeline
	running := &Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "release",
		Status:     "running",
		CurrentJob: new("lint"),
		StartedAt:  "20240115T103000Z",
		Jobs:       map[string]*JobData{},
	}
	err := SavePipeline(dir, "release-20240115T103000Z", running)
	require.NoError(t, err)

	// Create a completed pipeline
	completed := &Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "build",
		Status:     "completed",
		CurrentJob: nil,
		StartedAt:  "20240115T100000Z",
		EndedAt:    new("20240115T101000Z"),
		Jobs:       map[string]*JobData{},
	}
	err = SavePipeline(dir, "build-20240115T100000Z", completed)
	require.NoError(t, err)

	actives, warnings, err := ScanActivePipelines(dir)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, actives, 1)
	assert.Equal(t, "release-20240115T103000Z", actives[0].InstanceID)
	assert.Equal(t, "running", actives[0].Pipeline.Status)
}

func TestScanActivePipelinesCorruptFile(t *testing.T) {
	dir := t.TempDir()

	// Create a valid running pipeline
	running := &Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "release",
		Status:     "running",
		CurrentJob: new("lint"),
		StartedAt:  "20240115T103000Z",
		Jobs:       map[string]*JobData{},
	}
	err := SavePipeline(dir, "release-20240115T103000Z", running)
	require.NoError(t, err)

	// Write a corrupt YAML file
	err = os.WriteFile(filepath.Join(dir, "corrupt-20240115T110000Z.yaml"), []byte("{{invalid yaml"), 0o600)
	require.NoError(t, err)

	actives, warnings, err := ScanActivePipelines(dir)
	require.NoError(t, err)

	// Should still find the valid running pipeline
	require.Len(t, actives, 1)
	assert.Equal(t, "release-20240115T103000Z", actives[0].InstanceID)

	// Should have a warning for the corrupt file
	require.Len(t, warnings, 1)
	assert.Equal(t, "corrupt-20240115T110000Z", warnings[0].InstanceID)
	assert.Error(t, warnings[0].Err)
}

func TestScanActivePipelinesNonexistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")

	actives, warnings, err := ScanActivePipelines(dir)
	require.NoError(t, err)
	assert.Empty(t, actives)
	assert.Empty(t, warnings)
}

func TestScanActivePipelinesEmptyDir(t *testing.T) {
	dir := t.TempDir()

	actives, warnings, err := ScanActivePipelines(dir)
	require.NoError(t, err)
	assert.Empty(t, actives)
	assert.Empty(t, warnings)
}

func TestScanActivePipelinesMultipleRunning(t *testing.T) {
	dir := t.TempDir()

	for _, id := range []string{"a-20240115T103000Z", "b-20240115T104000Z"} {
		p := &Pipeline{
			Version:    "v0.1.0",
			WorkflowID: "test",
			Status:     "running",
			CurrentJob: new("job1"),
			StartedAt:  "20240115T103000Z",
			Jobs:       map[string]*JobData{},
		}
		err := SavePipeline(dir, id, p)
		require.NoError(t, err)
	}

	actives, warnings, err := ScanActivePipelines(dir)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Len(t, actives, 2)
}

func TestScanActivePipelinesSkipsNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a non-YAML file
	err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("not a pipeline"), 0o600)
	require.NoError(t, err)

	actives, warnings, err := ScanActivePipelines(dir)
	require.NoError(t, err)
	assert.Empty(t, actives)
	assert.Empty(t, warnings)
}
