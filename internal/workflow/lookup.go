package workflow

import (
	"path/filepath"

	"github.com/nextzhou/argus/internal/core"
)

// ExistsAtExpectedPath reports whether dir contains a parseable workflow at the canonical <id>.yaml path.
func ExistsAtExpectedPath(dir, id string) bool {
	if err := core.ValidateWorkflowID(id); err != nil {
		return false
	}

	path := filepath.Join(dir, core.ExpectedYAMLFileName(id))
	if err := core.ValidatePath(dir, path); err != nil {
		return false
	}

	wf, err := ParseWorkflowFile(path)
	if err != nil {
		return false
	}

	return wf.ID == id
}
