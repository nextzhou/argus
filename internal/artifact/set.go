// Package artifact provides scope-bound access to workflows, invariants,
// pipelines, and hook logs without leaking raw directory contracts.
package artifact

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nextzhou/argus/internal/core"
)

// WorkflowSummary contains list output for available workflows.
type WorkflowSummary struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Jobs        int    `json:"jobs"`
}

// Set aggregates artifact providers and stores for one resolved namespace.
type Set struct {
	workflows  *WorkflowProvider
	invariants *InvariantProvider
	pipelines  *PipelineStore
	hookLog    *HookLogStore
}

// NewProjectSet builds an artifact set backed by project-local Argus paths.
func NewProjectSet(projectRoot string) *Set {
	root := filepath.Join(projectRoot, ".argus")
	return newSet(
		projectRoot,
		filepath.Join(root, "workflows"),
		filepath.Join(root, "invariants"),
		filepath.Join(root, "pipelines"),
		filepath.Join(root, "logs", "hook.log"),
	)
}

// NewGlobalSet builds an artifact set backed by global Argus paths.
func NewGlobalSet(globalRoot, projectRoot string) *Set {
	return newSet(
		projectRoot,
		filepath.Join(globalRoot, "workflows"),
		filepath.Join(globalRoot, "invariants"),
		filepath.Join(globalRoot, "pipelines", core.ProjectPathToSafeID(projectRoot)),
		filepath.Join(globalRoot, "logs", "hook.log"),
	)
}

func newSet(projectRoot, workflowsDir, invariantsDir, pipelinesDir, hookLogPath string) *Set {
	return &Set{
		workflows:  NewWorkflowProvider(projectRoot, workflowsDir),
		invariants: NewInvariantProvider(projectRoot, invariantsDir),
		pipelines:  NewPipelineStore(projectRoot, pipelinesDir),
		hookLog:    NewHookLogStore(projectRoot, hookLogPath),
	}
}

// Workflows returns the workflow provider for the resolved scope.
func (s *Set) Workflows() *WorkflowProvider {
	if s == nil {
		return nil
	}
	return s.workflows
}

// Invariants returns the invariant provider for the resolved scope.
func (s *Set) Invariants() *InvariantProvider {
	if s == nil {
		return nil
	}
	return s.invariants
}

// Pipelines returns the pipeline store for the resolved scope.
func (s *Set) Pipelines() *PipelineStore {
	if s == nil {
		return nil
	}
	return s.pipelines
}

// HookLog returns the hook log store for the resolved scope.
func (s *Set) HookLog() *HookLogStore {
	if s == nil {
		return nil
	}
	return s.hookLog
}

// NewFallbackHookLogStore returns a hook log store rooted at the user config home.
func NewFallbackHookLogStore() (*HookLogStore, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	return NewHookLogStore("", filepath.Join(homeDir, ".config", "argus", "logs", "hook.log")), nil
}
