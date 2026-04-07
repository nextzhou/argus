package core

import (
	"fmt"
	"regexp"
	"strings"
)

// ID validation patterns.
const (
	// WorkflowIDPattern matches workflow and invariant IDs: lowercase, digits, hyphens.
	// Example valid: "my-workflow", "argus-init", "123-test"
	WorkflowIDPattern = `^[a-z0-9]+(-[a-z0-9]+)*$`

	// JobIDPattern matches job IDs: must start with letter, underscore separated.
	// Example valid: "run_tests", "build", "deploy_staging"
	JobIDPattern = `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`

	// SkillNamePattern matches skill names: same as workflow ID pattern.
	SkillNamePattern = `^[a-z0-9]+(-[a-z0-9]+)*$`

	// MaxSkillNameLength is the maximum length of a skill name.
	MaxSkillNameLength = 64

	// ArgusReservedPrefix is the prefix reserved for built-in Argus definitions.
	ArgusReservedPrefix = "argus-"
)

var (
	workflowIDRegex = regexp.MustCompile(WorkflowIDPattern)
	jobIDRegex      = regexp.MustCompile(JobIDPattern)
	skillNameRegex  = regexp.MustCompile(SkillNamePattern)
)

// ValidateWorkflowID validates a workflow or invariant ID.
// Valid: lowercase letters, digits, hyphens. Cannot start/end with hyphen.
// Examples: "my-workflow", "argus-init", "release-v2"
func ValidateWorkflowID(id string) error {
	if id == "" {
		return fmt.Errorf("workflow ID cannot be empty: %w", ErrInvalidID)
	}
	if !workflowIDRegex.MatchString(id) {
		return fmt.Errorf("workflow ID %q must match %s: %w", id, WorkflowIDPattern, ErrInvalidID)
	}
	return nil
}

// ValidateJobID validates a job ID within a workflow.
// Valid: starts with lowercase letter, then lowercase letters/digits, underscore separated.
// Examples: "run_tests", "build", "deploy_staging"
func ValidateJobID(id string) error {
	if id == "" {
		return fmt.Errorf("job ID cannot be empty: %w", ErrInvalidID)
	}
	if !jobIDRegex.MatchString(id) {
		return fmt.Errorf("job ID %q must match %s: %w", id, JobIDPattern, ErrInvalidID)
	}
	return nil
}

// ValidateSkillName validates an Agent Skill name.
// Valid: same pattern as workflow ID, max 64 characters, no colons.
// Examples: "argus-doctor", "my-skill", "lint-fixer"
func ValidateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("skill name cannot be empty: %w", ErrInvalidID)
	}
	if len(name) > MaxSkillNameLength {
		return fmt.Errorf("skill name %q exceeds maximum length of %d: %w", name, MaxSkillNameLength, ErrInvalidID)
	}
	if strings.Contains(name, ":") {
		return fmt.Errorf("skill name %q must not contain colon: %w", name, ErrInvalidID)
	}
	if !skillNameRegex.MatchString(name) {
		return fmt.Errorf("skill name %q must match %s: %w", name, SkillNamePattern, ErrInvalidID)
	}
	return nil
}

// IsArgusReserved reports whether the given ID uses the argus- reserved prefix.
func IsArgusReserved(id string) bool {
	return strings.HasPrefix(id, ArgusReservedPrefix)
}
