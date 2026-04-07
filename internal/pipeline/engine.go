package pipeline

import (
	"fmt"
	"strings"
	"time"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/workflow"
)

// Pipeline status values persisted in pipeline data files.
const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// AdvanceOpts controls how the current job advances the pipeline state machine.
type AdvanceOpts struct {
	Fail        bool
	EndPipeline bool
	Message     *string
	Now         time.Time
}

// CreatePipeline creates a new running pipeline, initializes the first job, and saves it.
//
// Phase 1 intentionally uses a non-atomic check-then-create flow. Concurrent starts can still
// race, but the current design accepts that limitation until file locking is introduced later.
func CreatePipeline(dir string, workflowID string, w *workflow.Workflow, now time.Time) (*Pipeline, string, error) {
	actives, _, err := ScanActivePipelines(dir)
	if err != nil {
		return nil, "", fmt.Errorf("scanning active pipelines: %w", err)
	}
	if len(actives) > 0 {
		return nil, "", fmt.Errorf("workflow %q cannot start while another pipeline is running: %w", workflowID, core.ErrActivePipelineExists)
	}
	if w == nil {
		return nil, "", fmt.Errorf("workflow %q is nil", workflowID)
	}
	if len(w.Jobs) == 0 {
		return nil, "", fmt.Errorf("workflow %q must have at least one job", workflowID)
	}

	firstJobID := w.Jobs[0].ID
	startedAt := core.FormatTimestamp(now)
	p := &Pipeline{
		Version:    core.SchemaVersion,
		WorkflowID: workflowID,
		Status:     StatusRunning,
		CurrentJob: stringPointer(firstJobID),
		StartedAt:  startedAt,
		Jobs: map[string]*JobData{
			firstJobID: {
				StartedAt: startedAt,
			},
		},
	}

	instanceID := NewInstanceID(workflowID, now)
	if err := SavePipeline(dir, instanceID, p); err != nil {
		return nil, "", fmt.Errorf("saving pipeline %q: %w", instanceID, err)
	}

	return p, instanceID, nil
}

// AdvanceJob applies the job-done state transitions defined by the pipeline state machine.
func AdvanceJob(p *Pipeline, w *workflow.Workflow, opts AdvanceOpts) error {
	if p == nil {
		return fmt.Errorf("pipeline is nil")
	}
	if w == nil {
		return fmt.Errorf("workflow %q is nil", p.WorkflowID)
	}
	if p.CurrentJob == nil {
		return fmt.Errorf("pipeline has no current_job: %w", core.ErrNoActivePipeline)
	}

	currentJobID := *p.CurrentJob
	currentJobIndex, found := FindJobIndex(w, currentJobID)
	if !found {
		return fmt.Errorf("current_job %q not found in workflow definition", currentJobID)
	}

	currentJobData, err := ensureCurrentJobData(p, currentJobID)
	if err != nil {
		return err
	}

	now := core.FormatTimestamp(opts.Now)
	currentJobData.EndedAt = stringPointer(now)
	if opts.Message != nil {
		currentJobData.Message = stringPointer(*opts.Message)
	}

	if opts.Fail {
		p.Status = StatusFailed
		p.EndedAt = stringPointer(now)
		return nil
	}

	if opts.EndPipeline || currentJobIndex == len(w.Jobs)-1 {
		p.Status = StatusCompleted
		p.CurrentJob = nil
		p.EndedAt = stringPointer(now)
		return nil
	}

	nextJobID := w.Jobs[currentJobIndex+1].ID
	ensureNextJobData(p, nextJobID, now)
	p.Status = StatusRunning
	p.CurrentJob = stringPointer(nextJobID)
	return nil
}

// CancelPipeline marks the pipeline as cancelled without changing current job runtime fields.
func CancelPipeline(p *Pipeline, now time.Time) {
	if p == nil {
		return
	}

	p.Status = StatusCancelled
	p.EndedAt = stringPointer(core.FormatTimestamp(now))
}

// DeriveJobStatus derives a job status from the workflow order and pipeline state.
func DeriveJobStatus(p *Pipeline, w *workflow.Workflow, jobID string) string {
	jobIndex, found := FindJobIndex(w, jobID)
	if !found {
		return ""
	}
	if p == nil {
		return "pending"
	}
	if p.Status == StatusCompleted {
		return "completed"
	}
	if p.CurrentJob == nil {
		return "pending"
	}

	currentIndex, found := FindJobIndex(w, *p.CurrentJob)
	if !found {
		return "pending"
	}

	switch {
	case jobIndex < currentIndex:
		return "completed"
	case jobIndex == currentIndex:
		return "in_progress"
	default:
		return "pending"
	}
}

// FindJobIndex returns the index of jobID within the workflow definition.
func FindJobIndex(w *workflow.Workflow, jobID string) (int, bool) {
	if w == nil {
		return -1, false
	}

	for index, job := range w.Jobs {
		if job.ID == jobID {
			return index, true
		}
	}

	return -1, false
}

func ensureCurrentJobData(p *Pipeline, currentJobID string) (*JobData, error) {
	if p.Jobs == nil {
		return nil, fmt.Errorf("current job %q data missing from pipeline", currentJobID)
	}

	jobData := p.Jobs[currentJobID]
	if jobData == nil {
		return nil, fmt.Errorf("current job %q data missing from pipeline", currentJobID)
	}

	return jobData, nil
}

func ensureNextJobData(p *Pipeline, nextJobID, now string) {
	if p.Jobs == nil {
		p.Jobs = make(map[string]*JobData)
	}

	nextJobData := p.Jobs[nextJobID]
	if nextJobData == nil {
		p.Jobs[nextJobID] = &JobData{StartedAt: now}
		return
	}
	if nextJobData.StartedAt == "" {
		nextJobData.StartedAt = now
	}
}

func stringPointer(value string) *string {
	cloned := strings.Clone(value)
	return &cloned
}
