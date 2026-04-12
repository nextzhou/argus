// Package scope resolves the active Argus scope and binds it to artifact access.
package scope

import "github.com/nextzhou/argus/internal/artifact"

// Kind identifies the resolved scope kind.
type Kind string

const (
	// KindProject identifies project-local scope.
	KindProject Kind = "project"
	// KindGlobal identifies workspace/global scope.
	KindGlobal Kind = "global"
)

// Resolved describes one active scope plus its bound artifact set.
type Resolved struct {
	kind        Kind
	projectRoot string
	artifacts   *artifact.Set
}

func newResolved(kind Kind, projectRoot string, artifacts *artifact.Set) *Resolved {
	return &Resolved{
		kind:        kind,
		projectRoot: projectRoot,
		artifacts:   artifacts,
	}
}

// NewProjectScope resolves a project-local scope with bound artifact access.
func NewProjectScope(projectRoot string) *Resolved {
	return newResolved(KindProject, projectRoot, artifact.NewProjectSet(projectRoot))
}

// NewGlobalScope resolves a workspace/global scope with bound artifact access.
func NewGlobalScope(globalRoot, projectRoot string) *Resolved {
	return newResolved(KindGlobal, projectRoot, artifact.NewGlobalSet(globalRoot, projectRoot))
}

// Kind returns the resolved scope kind.
func (r *Resolved) Kind() Kind {
	if r == nil {
		return ""
	}
	return r.kind
}

// ProjectRoot returns the current project root if one applies.
func (r *Resolved) ProjectRoot() string {
	if r == nil {
		return ""
	}
	return r.projectRoot
}

// Artifacts returns the scope-bound artifact set.
func (r *Resolved) Artifacts() *artifact.Set {
	if r == nil {
		return nil
	}
	return r.artifacts
}
