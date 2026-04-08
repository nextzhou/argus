// Package assets provides access to embedded asset files bundled into the argus binary.
package assets

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed skills workflows invariants prompts hooks
var embedded embed.FS

// ReadAsset reads a single embedded asset file by path (relative to assets root).
// Example: ReadAsset("workflows/argus-init.yaml")
func ReadAsset(path string) ([]byte, error) {
	data, err := embedded.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading asset %q: %w", path, err)
	}
	return data, nil
}

// ListAssets returns the names of entries in a subdirectory.
// Example: ListAssets("skills") returns ["argus-doctor", "argus-install", ...]
func ListAssets(subdir string) ([]string, error) {
	entries, err := embedded.ReadDir(subdir)
	if err != nil {
		return nil, fmt.Errorf("listing assets in %q: %w", subdir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// WalkAssets walks the embedded filesystem under subdir, calling fn for each entry.
func WalkAssets(subdir string, fn fs.WalkDirFunc) error {
	return fs.WalkDir(embedded, subdir, fn)
}
