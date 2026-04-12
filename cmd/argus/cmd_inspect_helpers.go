package main

import (
	"fmt"

	"github.com/nextzhou/argus/internal/artifact"
	"github.com/nextzhou/argus/internal/assets"
)

func builtinWorkflowAllowReservedID() (func(string) bool, error) {
	ids, err := assets.BuiltinWorkflowIDs()
	if err != nil {
		return nil, fmt.Errorf("loading built-in workflows: %w", err)
	}
	return allowReservedIDs(ids), nil
}

func builtinInvariantAllowReservedID() (func(string) bool, error) {
	ids, err := assets.BuiltinInvariantIDs()
	if err != nil {
		return nil, fmt.Errorf("loading built-in invariants: %w", err)
	}
	return allowReservedIDs(ids), nil
}

func allowReservedIDs(ids map[string]struct{}) func(string) bool {
	return func(id string) bool {
		_, ok := ids[id]
		return ok
	}
}

func buildWorkflowChecker(projectRoot, workflowsDir string) func(id string) bool {
	provider := artifact.NewWorkflowProvider(projectRoot, workflowsDir)
	return func(id string) bool {
		return provider.Exists(id)
	}
}
