package artifact

import (
	"fmt"
	"time"

	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/workflow"
)

// PipelineStore provides persisted pipeline state for one namespace.
type PipelineStore struct {
	projectRoot string
	dir         string
}

// NewPipelineStore creates a pipeline store for one artifact namespace.
func NewPipelineStore(projectRoot, dir string) *PipelineStore {
	return &PipelineStore{
		projectRoot: projectRoot,
		dir:         dir,
	}
}

// ProjectRoot returns the project root used for relative rendering and policy.
func (s *PipelineStore) ProjectRoot() string {
	if s == nil {
		return ""
	}
	return s.projectRoot
}

// Create creates and persists a new pipeline instance for the workflow.
func (s *PipelineStore) Create(workflowID string, wf *workflow.Workflow, now time.Time) (*pipeline.Pipeline, string, error) {
	if s == nil {
		return nil, "", fmt.Errorf("pipeline store is nil")
	}
	return pipeline.CreatePipeline(s.dir, workflowID, wf, now)
}

// Save persists pipeline state for an existing instance.
func (s *PipelineStore) Save(instanceID string, p *pipeline.Pipeline) error {
	if s == nil {
		return fmt.Errorf("pipeline store is nil")
	}
	return pipeline.SavePipeline(s.dir, instanceID, p)
}

// Load loads pipeline state for one instance.
func (s *PipelineStore) Load(instanceID string) (*pipeline.Pipeline, error) {
	if s == nil {
		return nil, fmt.Errorf("pipeline store is nil")
	}
	return pipeline.LoadPipeline(s.dir, instanceID)
}

// ScanActive scans for currently running pipeline instances.
func (s *PipelineStore) ScanActive() ([]pipeline.ActivePipeline, []pipeline.ScanWarning, error) {
	if s == nil {
		return nil, nil, fmt.Errorf("pipeline store is nil")
	}
	return pipeline.ScanActivePipelines(s.dir)
}
