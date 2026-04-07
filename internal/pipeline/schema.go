// Package pipeline provides pipeline data schema and YAML persistence.
package pipeline

// Pipeline represents the runtime state of a workflow execution instance.
type Pipeline struct {
	Version    string              `yaml:"version"`
	WorkflowID string              `yaml:"workflow_id"`
	Status     string              `yaml:"status"`
	CurrentJob *string             `yaml:"current_job"`
	StartedAt  string              `yaml:"started_at"`
	EndedAt    *string             `yaml:"ended_at"`
	Jobs       map[string]*JobData `yaml:"jobs"`
}

// JobData records runtime outputs for a single job execution.
type JobData struct {
	StartedAt string  `yaml:"started_at"`
	EndedAt   *string `yaml:"ended_at"`
	Message   *string `yaml:"message"`
}
