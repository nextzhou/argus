package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPipelineZeroValue(t *testing.T) {
	var p Pipeline

	assert.Empty(t, p.Version)
	assert.Empty(t, p.WorkflowID)
	assert.Empty(t, p.Status)
	assert.Nil(t, p.CurrentJob)
	assert.Empty(t, p.StartedAt)
	assert.Nil(t, p.EndedAt)
	assert.Nil(t, p.Jobs)
}

func TestJobDataZeroValue(t *testing.T) {
	var j JobData

	assert.Empty(t, j.StartedAt)
	assert.Nil(t, j.EndedAt)
	assert.Nil(t, j.Message)
}

func TestPipelineFields(t *testing.T) {
	currentJob := "run_tests"
	endedAt := "20240115T103200Z"
	msg := "All lint checks passed"

	p := Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "release",
		Status:     "running",
		CurrentJob: &currentJob,
		StartedAt:  "20240115T103000Z",
		EndedAt:    &endedAt,
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

	assert.Equal(t, "v0.1.0", p.Version)
	assert.Equal(t, "release", p.WorkflowID)
	assert.Equal(t, "running", p.Status)
	assert.Equal(t, "run_tests", *p.CurrentJob)
	assert.Equal(t, "20240115T103000Z", p.StartedAt)
	assert.Equal(t, "20240115T103200Z", *p.EndedAt)
	assert.Len(t, p.Jobs, 2)

	lint := p.Jobs["lint"]
	assert.Equal(t, "20240115T103005Z", lint.StartedAt)
	assert.Equal(t, &endedAt, lint.EndedAt)
	assert.Equal(t, &msg, lint.Message)

	runTests := p.Jobs["run_tests"]
	assert.Equal(t, "20240115T103105Z", runTests.StartedAt)
	assert.Nil(t, runTests.EndedAt)
	assert.Nil(t, runTests.Message)
}

func TestPipelineNullableFields(t *testing.T) {
	tests := []struct {
		name       string
		currentJob *string
		endedAt    *string
	}{
		{
			name:       "running pipeline with current job",
			currentJob: strPtr("build"),
			endedAt:    nil,
		},
		{
			name:       "completed pipeline with null current job",
			currentJob: nil,
			endedAt:    strPtr("20240115T104000Z"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Pipeline{
				Version:    "v0.1.0",
				WorkflowID: "test",
				Status:     "running",
				CurrentJob: tt.currentJob,
				StartedAt:  "20240115T103000Z",
				EndedAt:    tt.endedAt,
				Jobs:       map[string]*JobData{},
			}

			assert.Equal(t, tt.currentJob, p.CurrentJob)
			assert.Equal(t, tt.endedAt, p.EndedAt)
		})
	}
}

func strPtr(s string) *string {
	return &s
}
