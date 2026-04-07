// Package workspace provides project root discovery and workspace utilities.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// ProjectRoot represents a discovered project root directory.
type ProjectRoot struct {
	Path     string // Absolute path to the project root directory
	HasArgus bool   // .argus/ directory exists at this path
	HasGit   bool   // .git/ directory exists at this path
}

// FindProjectRoot searches upward from cwd for a project root.
// It first looks for .argus/ directory (priority), then falls back to .git/.
// Returns nil, nil if neither is found (not an error).
// Returns nil, error only if path normalization fails.
func FindProjectRoot(cwd string) (*ProjectRoot, error) {
	// Normalize the starting path
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolving cwd: %w", err)
	}

	// Search upward for .argus/ first (priority)
	current := abs
	for {
		argusPath := filepath.Join(current, ".argus")
		gitPath := filepath.Join(current, ".git")

		// Check if .argus/ exists
		if isDir(argusPath) {
			// Found .argus/, check if .git/ also exists at this level
			hasGit := isDir(gitPath)
			return &ProjectRoot{
				Path:     current,
				HasArgus: true,
				HasGit:   hasGit,
			}, nil
		}

		// Move to parent directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root
			break
		}
		current = parent
	}

	// .argus/ not found, search upward for .git/ as fallback
	current = abs
	for {
		gitPath := filepath.Join(current, ".git")

		// Check if .git/ exists
		if isDir(gitPath) {
			return &ProjectRoot{
				Path:     current,
				HasArgus: false,
				HasGit:   true,
			}, nil
		}

		// Move to parent directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root
			break
		}
		current = parent
	}

	// Neither .argus/ nor .git/ found
	return nil, nil
}

// isDir checks if a path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// IsSubdirectory checks if child path is under parent path using segment-based matching.
// Returns true if child is a subdirectory of parent or equal to parent.
func IsSubdirectory(parent, child string) bool {
	// Normalize both paths to absolute
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	childAbs, err := filepath.Abs(child)
	if err != nil {
		return false
	}

	// Clean paths
	parentAbs = filepath.Clean(parentAbs)
	childAbs = filepath.Clean(childAbs)

	// Check exact match
	if parentAbs == childAbs {
		return true
	}

	// Check if child starts with parent + separator
	return len(childAbs) > len(parentAbs) &&
		childAbs[:len(parentAbs)] == parentAbs &&
		childAbs[len(parentAbs)] == filepath.Separator
}
