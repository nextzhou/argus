package workflow

import (
	"fmt"
	"io"
	"os"

	"github.com/nextzhou/argus/internal/core"
	"gopkg.in/yaml.v3"
)

// ParseWorkflow decodes and validates a workflow definition from the given reader.
// Unknown YAML fields are rejected.
func ParseWorkflow(r io.Reader) (*Workflow, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)

	var w Workflow
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("parsing workflow YAML: %w", err)
	}

	if err := validateWorkflow(&w); err != nil {
		return nil, err
	}

	return &w, nil
}

// ParseWorkflowFile parses a workflow definition from the file at the given path.
func ParseWorkflowFile(path string) (*Workflow, error) {
	//nolint:gosec // ParseWorkflowFile intentionally reads the exact file path selected by the caller.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening workflow file: %w", err)
	}
	defer func() { _ = f.Close() }()

	w, err := ParseWorkflow(f)
	if err != nil {
		return nil, fmt.Errorf("parsing workflow file %q: %w", path, err)
	}
	return w, nil
}

func validateWorkflow(w *Workflow) error {
	if err := core.CheckCompatibility(w.Version); err != nil {
		return fmt.Errorf("workflow version check: %w", err)
	}

	if w.ID == "" {
		return fmt.Errorf("workflow ID cannot be empty: %w", core.ErrInvalidID)
	}
	if err := core.ValidateWorkflowID(w.ID); err != nil {
		return fmt.Errorf("workflow %q: %w", w.ID, err)
	}

	if len(w.Jobs) == 0 {
		return fmt.Errorf("workflow %q must have at least one job", w.ID)
	}

	for i, job := range w.Jobs {
		if err := validateJob(w.ID, i, &job); err != nil {
			return err
		}
	}

	return nil
}

func validateJob(workflowID string, index int, job *Job) error {
	if job.ID != "" {
		if err := core.ValidateJobID(job.ID); err != nil {
			return fmt.Errorf("workflow %q job[%d]: %w", workflowID, index, err)
		}
	}

	if job.Ref == "" && job.Prompt == "" && job.Skill == "" {
		jobName := fmt.Sprintf("job[%d]", index)
		if job.ID != "" {
			jobName = fmt.Sprintf("job %q (index %d)", job.ID, index)
		}
		return fmt.Errorf("workflow %q %s must have a prompt or skill", workflowID, jobName)
	}

	return nil
}
