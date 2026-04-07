// Package invariant provides YAML schema definitions, parsing, and validation for Argus invariant files.
package invariant

// Invariant represents a parsed invariant definition file.
type Invariant struct {
	Version     string      `yaml:"version"`
	ID          string      `yaml:"id"`
	Description string      `yaml:"description,omitempty"`
	Auto        string      `yaml:"auto,omitempty"`
	Check       []CheckStep `yaml:"check"`
	Prompt      string      `yaml:"prompt,omitempty"`
	Workflow    string      `yaml:"workflow,omitempty"`
}

// CheckStep represents one shell check step.
type CheckStep struct {
	Shell       string `yaml:"shell"`
	Description string `yaml:"description,omitempty"`
}
