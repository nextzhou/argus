package assets

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/workflow"
)

// BuiltinWorkflowIDs returns the IDs of workflows embedded in the current Argus binary.
func BuiltinWorkflowIDs() (map[string]struct{}, error) {
	return builtinIDs("workflows", func(path string, data []byte) (string, error) {
		wf, err := workflow.ParseWorkflow(bytes.NewReader(data))
		if err != nil {
			return "", fmt.Errorf("parsing embedded workflow %q: %w", path, err)
		}
		return wf.ID, nil
	})
}

// BuiltinInvariantIDs returns the IDs of invariants embedded in the current Argus binary.
func BuiltinInvariantIDs() (map[string]struct{}, error) {
	return builtinIDs("invariants", func(path string, data []byte) (string, error) {
		inv, err := invariant.ParseInvariant(bytes.NewReader(data))
		if err != nil {
			return "", fmt.Errorf("parsing embedded invariant %q: %w", path, err)
		}
		return inv.ID, nil
	})
}

func builtinIDs(subdir string, extractID func(path string, data []byte) (string, error)) (map[string]struct{}, error) {
	names, err := ListAssets(subdir)
	if err != nil {
		return nil, fmt.Errorf("listing embedded %s: %w", subdir, err)
	}

	ids := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}

		path := filepath.Join(subdir, name)
		data, err := ReadAsset(path)
		if err != nil {
			return nil, err
		}
		id, err := extractID(path, data)
		if err != nil {
			return nil, err
		}
		ids[id] = struct{}{}
	}

	return ids, nil
}
