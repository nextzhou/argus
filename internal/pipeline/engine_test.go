package pipeline_test

import (
	"testing"
	"time"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineTransitions(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	message := "done"

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "create pipeline initializes first job",
			run: func(t *testing.T) {
				dir := t.TempDir()
				wf := testWorkflow("lint", "test")

				p, instanceID, err := pipeline.CreatePipeline(dir, wf.ID, wf, now)
				require.NoError(t, err)
				assert.Equal(t, pipeline.NewInstanceID(wf.ID, now), instanceID)
				assert.Equal(t, pipeline.StatusRunning, p.Status)
				require.NotNil(t, p.CurrentJob)
				assert.Equal(t, "lint", *p.CurrentJob)
				require.Contains(t, p.Jobs, "lint")
				assert.Equal(t, core.FormatTimestamp(now), p.Jobs["lint"].StartedAt)
			},
		},
		{
			name: "create pipeline rejects active pipeline",
			run: func(t *testing.T) {
				dir := t.TempDir()
				wf := testWorkflow("lint", "test")
				writeRunningPipeline(t, dir, now.Add(-time.Minute))

				_, _, err := pipeline.CreatePipeline(dir, wf.ID, wf, now)
				require.Error(t, err)
				assert.ErrorIs(t, err, core.ErrActivePipelineExists)
			},
		},
		{
			name: "advance middle job starts next job",
			run: func(t *testing.T) {
				wf := testWorkflow("lint", "test", "deploy")
				p := testRunningPipeline(now, wf, "lint")

				err := pipeline.AdvanceJob(p, wf, pipeline.AdvanceOpts{Message: &message, Now: now.Add(time.Minute)})
				require.NoError(t, err)
				assert.Equal(t, pipeline.StatusRunning, p.Status)
				require.NotNil(t, p.CurrentJob)
				assert.Equal(t, "test", *p.CurrentJob)
				require.NotNil(t, p.Jobs["lint"].EndedAt)
				assert.Equal(t, core.FormatTimestamp(now.Add(time.Minute)), *p.Jobs["lint"].EndedAt)
				require.NotNil(t, p.Jobs["lint"].Message)
				assert.Equal(t, message, *p.Jobs["lint"].Message)
				require.Contains(t, p.Jobs, "test")
				assert.Equal(t, core.FormatTimestamp(now.Add(time.Minute)), p.Jobs["test"].StartedAt)
			},
		},
		{
			name: "advance last job completes pipeline",
			run: func(t *testing.T) {
				wf := testWorkflow("lint")
				p := testRunningPipeline(now, wf, "lint")

				err := pipeline.AdvanceJob(p, wf, pipeline.AdvanceOpts{Message: &message, Now: now.Add(time.Minute)})
				require.NoError(t, err)
				assert.Equal(t, pipeline.StatusCompleted, p.Status)
				assert.Nil(t, p.CurrentJob)
				require.NotNil(t, p.EndedAt)
				assert.Equal(t, core.FormatTimestamp(now.Add(time.Minute)), *p.EndedAt)
			},
		},
		{
			name: "advance with end pipeline completes early",
			run: func(t *testing.T) {
				wf := testWorkflow("lint", "test")
				p := testRunningPipeline(now, wf, "lint")

				err := pipeline.AdvanceJob(p, wf, pipeline.AdvanceOpts{EndPipeline: true, Message: &message, Now: now.Add(time.Minute)})
				require.NoError(t, err)
				assert.Equal(t, pipeline.StatusCompleted, p.Status)
				assert.Nil(t, p.CurrentJob)
				require.NotNil(t, p.EndedAt)
				assert.Equal(t, core.FormatTimestamp(now.Add(time.Minute)), *p.EndedAt)
				assert.NotContains(t, p.Jobs, "test")
			},
		},
		{
			name: "advance with fail marks pipeline failed",
			run: func(t *testing.T) {
				wf := testWorkflow("lint", "test")
				p := testRunningPipeline(now, wf, "lint")

				err := pipeline.AdvanceJob(p, wf, pipeline.AdvanceOpts{Fail: true, Message: &message, Now: now.Add(time.Minute)})
				require.NoError(t, err)
				assert.Equal(t, pipeline.StatusFailed, p.Status)
				require.NotNil(t, p.CurrentJob)
				assert.Equal(t, "lint", *p.CurrentJob)
				require.NotNil(t, p.EndedAt)
				assert.Equal(t, core.FormatTimestamp(now.Add(time.Minute)), *p.EndedAt)
			},
		},
		{
			name: "advance with fail and end pipeline behaves as failure",
			run: func(t *testing.T) {
				wf := testWorkflow("lint", "test")
				p := testRunningPipeline(now, wf, "lint")

				err := pipeline.AdvanceJob(p, wf, pipeline.AdvanceOpts{Fail: true, EndPipeline: true, Message: &message, Now: now.Add(time.Minute)})
				require.NoError(t, err)
				assert.Equal(t, pipeline.StatusFailed, p.Status)
				require.NotNil(t, p.CurrentJob)
				assert.Equal(t, "lint", *p.CurrentJob)
				require.NotNil(t, p.EndedAt)
				assert.Equal(t, core.FormatTimestamp(now.Add(time.Minute)), *p.EndedAt)
			},
		},
		{
			name: "advance returns error when current job missing from workflow",
			run: func(t *testing.T) {
				wf := testWorkflow("lint", "test")
				p := testRunningPipeline(now, wf, "missing")

				err := pipeline.AdvanceJob(p, wf, pipeline.AdvanceOpts{Now: now.Add(time.Minute)})
				require.Error(t, err)
				assert.Contains(t, err.Error(), "current_job \"missing\" not found in workflow definition")
			},
		},
		{
			name: "cancel pipeline preserves current job runtime data",
			run: func(t *testing.T) {
				wf := testWorkflow("lint", "test")
				p := testRunningPipeline(now, wf, "lint")

				pipeline.CancelPipeline(p, now.Add(2*time.Minute))

				assert.Equal(t, pipeline.StatusCancelled, p.Status)
				require.NotNil(t, p.CurrentJob)
				assert.Equal(t, "lint", *p.CurrentJob)
				require.NotNil(t, p.EndedAt)
				assert.Equal(t, core.FormatTimestamp(now.Add(2*time.Minute)), *p.EndedAt)
				assert.Nil(t, p.Jobs["lint"].EndedAt)
				assert.Nil(t, p.Jobs["lint"].Message)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

func TestCreatePipeline(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		wantErr error
	}{
		{
			name: "creates and persists first job",
			setup: func(t *testing.T, _ string) {
				t.Helper()
			},
		},
		{
			name: "returns active pipeline exists when running pipeline already present",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeRunningPipeline(t, dir, now.Add(-time.Minute))
			},
			wantErr: core.ErrActivePipelineExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			wf := testWorkflow("lint", "test")
			tt.setup(t, dir)

			p, instanceID, err := pipeline.CreatePipeline(dir, wf.ID, wf, now)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, p)
				assert.Empty(t, instanceID)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, core.SchemaVersion, p.Version)
			assert.Equal(t, wf.ID, p.WorkflowID)
			assert.Equal(t, pipeline.StatusRunning, p.Status)
			assert.Equal(t, core.FormatTimestamp(now), p.StartedAt)
			assert.Nil(t, p.EndedAt)
			require.NotNil(t, p.CurrentJob)
			assert.Equal(t, "lint", *p.CurrentJob)

			loaded, loadErr := pipeline.LoadPipeline(dir, instanceID)
			require.NoError(t, loadErr)
			assert.Equal(t, p.Version, loaded.Version)
			assert.Equal(t, p.WorkflowID, loaded.WorkflowID)
			assert.Equal(t, p.Status, loaded.Status)
			assert.Equal(t, *p.CurrentJob, *loaded.CurrentJob)
			assert.Equal(t, p.StartedAt, loaded.StartedAt)
			assert.Equal(t, p.EndedAt, loaded.EndedAt)
			require.Contains(t, loaded.Jobs, "lint")
			assert.Equal(t, core.FormatTimestamp(now), loaded.Jobs["lint"].StartedAt)
		})
	}
}

func TestAdvanceJob(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	message := "job finished"

	buildThreeJobWorkflow := func() *workflow.Workflow {
		return testWorkflow("lint", "test", "deploy")
	}

	tests := []struct {
		name             string
		workflow         func() *workflow.Workflow
		pipeline         func() *pipeline.Pipeline
		opts             pipeline.AdvanceOpts
		wantStatus       string
		wantCurrentJob   *string
		wantPipelineEnd  bool
		wantStartedJobID string
		wantErr          string
	}{
		{
			name:     "middle job advances to next job",
			workflow: buildThreeJobWorkflow,
			pipeline: func() *pipeline.Pipeline {
				wf := buildThreeJobWorkflow()
				return testRunningPipeline(now, wf, "lint")
			},
			opts:             pipeline.AdvanceOpts{Message: &message, Now: now.Add(time.Minute)},
			wantStatus:       pipeline.StatusRunning,
			wantCurrentJob:   stringPtr("test"),
			wantStartedJobID: "test",
		},
		{
			name:     "last job completes pipeline",
			workflow: func() *workflow.Workflow { return testWorkflow("lint") },
			pipeline: func() *pipeline.Pipeline {
				wf := testWorkflow("lint")
				return testRunningPipeline(now, wf, "lint")
			},
			opts:            pipeline.AdvanceOpts{Message: &message, Now: now.Add(time.Minute)},
			wantStatus:      pipeline.StatusCompleted,
			wantPipelineEnd: true,
		},
		{
			name:     "end pipeline completes early",
			workflow: buildThreeJobWorkflow,
			pipeline: func() *pipeline.Pipeline {
				wf := buildThreeJobWorkflow()
				return testRunningPipeline(now, wf, "lint")
			},
			opts:            pipeline.AdvanceOpts{EndPipeline: true, Message: &message, Now: now.Add(time.Minute)},
			wantStatus:      pipeline.StatusCompleted,
			wantPipelineEnd: true,
		},
		{
			name:     "fail preserves current job",
			workflow: buildThreeJobWorkflow,
			pipeline: func() *pipeline.Pipeline {
				wf := buildThreeJobWorkflow()
				return testRunningPipeline(now, wf, "lint")
			},
			opts:            pipeline.AdvanceOpts{Fail: true, Message: &message, Now: now.Add(time.Minute)},
			wantStatus:      pipeline.StatusFailed,
			wantCurrentJob:  stringPtr("lint"),
			wantPipelineEnd: true,
		},
		{
			name:     "fail and end pipeline behaves as fail",
			workflow: buildThreeJobWorkflow,
			pipeline: func() *pipeline.Pipeline {
				wf := buildThreeJobWorkflow()
				return testRunningPipeline(now, wf, "lint")
			},
			opts:            pipeline.AdvanceOpts{Fail: true, EndPipeline: true, Message: &message, Now: now.Add(time.Minute)},
			wantStatus:      pipeline.StatusFailed,
			wantCurrentJob:  stringPtr("lint"),
			wantPipelineEnd: true,
		},
		{
			name:     "returns error when current job missing from workflow",
			workflow: buildThreeJobWorkflow,
			pipeline: func() *pipeline.Pipeline {
				wf := buildThreeJobWorkflow()
				return testRunningPipeline(now, wf, "missing")
			},
			opts:    pipeline.AdvanceOpts{Now: now.Add(time.Minute)},
			wantErr: "current_job \"missing\" not found in workflow definition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := tt.workflow()
			p := tt.pipeline()

			err := pipeline.AdvanceJob(p, wf, tt.opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, p.Status)
			assert.Equal(t, tt.wantCurrentJob, p.CurrentJob)
			require.NotNil(t, p.Jobs["lint"].EndedAt)
			assert.Equal(t, core.FormatTimestamp(tt.opts.Now), *p.Jobs["lint"].EndedAt)
			require.NotNil(t, p.Jobs["lint"].Message)
			assert.Equal(t, message, *p.Jobs["lint"].Message)

			if tt.wantPipelineEnd {
				require.NotNil(t, p.EndedAt)
				assert.Equal(t, core.FormatTimestamp(tt.opts.Now), *p.EndedAt)
			} else {
				assert.Nil(t, p.EndedAt)
			}

			if tt.wantStartedJobID != "" {
				require.Contains(t, p.Jobs, tt.wantStartedJobID)
				assert.Equal(t, core.FormatTimestamp(tt.opts.Now), p.Jobs[tt.wantStartedJobID].StartedAt)
			}
		})
	}
}

func TestCancelPipeline(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	p := testRunningPipeline(now, testWorkflow("lint", "test"), "lint")

	pipeline.CancelPipeline(p, now.Add(5*time.Minute))

	assert.Equal(t, pipeline.StatusCancelled, p.Status)
	require.NotNil(t, p.CurrentJob)
	assert.Equal(t, "lint", *p.CurrentJob)
	require.NotNil(t, p.EndedAt)
	assert.Equal(t, core.FormatTimestamp(now.Add(5*time.Minute)), *p.EndedAt)
	assert.Nil(t, p.Jobs["lint"].EndedAt)
	assert.Nil(t, p.Jobs["lint"].Message)
}

func TestDeriveJobStatus(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	wf := testWorkflow("lint", "test", "deploy")

	tests := []struct {
		name  string
		p     *pipeline.Pipeline
		jobID string
		want  string
	}{
		{
			name:  "running pipeline marks previous jobs completed",
			p:     testRunningPipeline(now, wf, "test"),
			jobID: "lint",
			want:  "completed",
		},
		{
			name:  "running pipeline marks current job in progress",
			p:     testRunningPipeline(now, wf, "test"),
			jobID: "test",
			want:  "in_progress",
		},
		{
			name:  "running pipeline marks later jobs pending",
			p:     testRunningPipeline(now, wf, "test"),
			jobID: "deploy",
			want:  "pending",
		},
		{
			name: "completed pipeline marks all jobs completed",
			p: &pipeline.Pipeline{
				Status:     pipeline.StatusCompleted,
				CurrentJob: nil,
			},
			jobID: "deploy",
			want:  "completed",
		},
		{
			name:  "failed pipeline keeps current job in progress",
			p:     testTerminalPipeline(pipeline.StatusFailed, now, wf, "test"),
			jobID: "test",
			want:  "in_progress",
		},
		{
			name:  "cancelled pipeline keeps future jobs pending",
			p:     testTerminalPipeline(pipeline.StatusCancelled, now, wf, "test"),
			jobID: "deploy",
			want:  "pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pipeline.DeriveJobStatus(tt.p, wf, tt.jobID))
		})
	}
}

func TestFindJobIndex(t *testing.T) {
	wf := testWorkflow("lint", "test", "deploy")

	tests := []struct {
		name      string
		jobID     string
		want      int
		wantFound bool
	}{
		{name: "first job", jobID: "lint", want: 0, wantFound: true},
		{name: "middle job", jobID: "test", want: 1, wantFound: true},
		{name: "missing job", jobID: "missing", want: -1, wantFound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := pipeline.FindJobIndex(wf, tt.jobID)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantFound, found)
		})
	}
}

func testWorkflow(jobIDs ...string) *workflow.Workflow {
	jobs := make([]workflow.Job, 0, len(jobIDs))
	for _, jobID := range jobIDs {
		jobs = append(jobs, workflow.Job{ID: jobID})
	}

	return &workflow.Workflow{
		Version: core.SchemaVersion,
		ID:      "release",
		Jobs:    jobs,
	}
}

func testRunningPipeline(now time.Time, wf *workflow.Workflow, currentJob string) *pipeline.Pipeline {
	p := &pipeline.Pipeline{
		Version:    core.SchemaVersion,
		WorkflowID: wf.ID,
		Status:     pipeline.StatusRunning,
		CurrentJob: &currentJob,
		StartedAt:  core.FormatTimestamp(now),
		Jobs:       make(map[string]*pipeline.JobData, len(wf.Jobs)),
	}

	if index, found := pipeline.FindJobIndex(wf, currentJob); found {
		for _, job := range wf.Jobs[:index] {
			completedAt := core.FormatTimestamp(now.Add(-time.Minute))
			p.Jobs[job.ID] = &pipeline.JobData{
				StartedAt: core.FormatTimestamp(now.Add(-2 * time.Minute)),
				EndedAt:   &completedAt,
			}
		}
	}

	p.Jobs[currentJob] = &pipeline.JobData{StartedAt: core.FormatTimestamp(now)}
	return p
}

func testTerminalPipeline(status string, now time.Time, wf *workflow.Workflow, currentJob string) *pipeline.Pipeline {
	p := testRunningPipeline(now, wf, currentJob)
	p.Status = status
	p.EndedAt = stringPtr(core.FormatTimestamp(now.Add(time.Minute)))
	return p
}

func writeRunningPipeline(t *testing.T, dir string, now time.Time) {
	t.Helper()
	err := pipeline.SavePipeline(dir, "existing-20240115T102000Z", &pipeline.Pipeline{
		Version:    core.SchemaVersion,
		WorkflowID: "existing",
		Status:     pipeline.StatusRunning,
		CurrentJob: stringPtr("job1"),
		StartedAt:  core.FormatTimestamp(now),
		Jobs: map[string]*pipeline.JobData{
			"job1": {StartedAt: core.FormatTimestamp(now)},
		},
	})
	require.NoError(t, err)
}

func stringPtr(s string) *string {
	return &s
}
