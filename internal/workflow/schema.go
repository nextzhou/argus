// Package workflow provides YAML schema definitions and parsing for Argus workflow files.
package workflow

// Workflow represents a parsed workflow definition file.
type Workflow struct {
	Version     string `yaml:"version"`
	ID          string `yaml:"id"`
	Description string `yaml:"description,omitempty"`
	Jobs        []Job  `yaml:"jobs"`
}

// Job represents a single step in a workflow.
type Job struct {
	ID          string `yaml:"id,omitempty"`
	Description string `yaml:"description,omitempty"`
	Ref         string `yaml:"ref,omitempty"`
	Prompt      string `yaml:"prompt,omitempty"`
	Skill       string `yaml:"skill,omitempty"`
}
