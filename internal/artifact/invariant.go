package artifact

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/nextzhou/argus/internal/invariant"
)

// InvariantProvider provides invariant artifacts for one namespace.
type InvariantProvider struct {
	projectRoot string
	dir         string
}

// NewInvariantProvider creates an invariant provider for one artifact namespace.
func NewInvariantProvider(projectRoot, dir string) *InvariantProvider {
	return &InvariantProvider{
		projectRoot: projectRoot,
		dir:         dir,
	}
}

// ProjectRoot returns the project root used for relative rendering and policy.
func (p *InvariantProvider) ProjectRoot() string {
	if p == nil {
		return ""
	}
	return p.projectRoot
}

// Dir returns the backing invariant directory.
func (p *InvariantProvider) Dir() string {
	if p == nil {
		return ""
	}
	return p.dir
}

// Catalog loads the invariant catalog visible through this provider.
func (p *InvariantProvider) Catalog(ignoreUnderscore bool) (*invariant.Catalog, error) {
	if p == nil {
		return nil, fmt.Errorf("invariant provider is nil")
	}
	catalog, err := invariant.LoadCatalog(p.dir, ignoreUnderscore)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return invariant.EmptyCatalog(), nil
		}
		return nil, fmt.Errorf("loading invariant catalog: %w", err)
	}
	return catalog, nil
}

// Inspect validates all invariants visible through this provider.
func (p *InvariantProvider) Inspect(workflowChecker func(id string) bool, allowReservedID func(id string) bool) (*invariant.InspectReport, error) {
	if p == nil {
		return nil, fmt.Errorf("invariant provider is nil")
	}
	return invariant.InspectDirectory(p.projectRoot, p.dir, workflowChecker, allowReservedID)
}
