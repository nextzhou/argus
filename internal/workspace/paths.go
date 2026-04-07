package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NormalizePath converts any path (relative, absolute, tilde) to a canonical
// form using a 4-step algorithm:
//  1. Resolve to absolute (expand ~ or use CWD for relative)
//  2. Clean (eliminate . / .. / trailing slashes)
//  3. Compress HOME prefix back to ~
//  4. Return result
func NormalizePath(input string) (string, error) {
	home := os.Getenv("HOME")

	var abs string
	if strings.HasPrefix(input, "~/") || input == "~" {
		abs = home + input[1:]
	} else {
		var err error
		abs, err = filepath.Abs(input)
		if err != nil {
			return "", fmt.Errorf("resolving path: %w", err)
		}
	}

	clean := filepath.Clean(abs)

	if home != "" && (clean == home || strings.HasPrefix(clean, home+string(filepath.Separator))) {
		clean = "~" + clean[len(home):]
	}

	return clean, nil
}

// ExpandPath replaces a leading ~ with the HOME directory.
func ExpandPath(stored string) string {
	if stored == "~" {
		return os.Getenv("HOME")
	}
	if strings.HasPrefix(stored, "~/") {
		return os.Getenv("HOME") + stored[1:]
	}
	return stored
}

// IsInWorkspace returns true if projectPath falls under any workspace using segment-based prefix matching.
func IsInWorkspace(projectPath string, workspaces []string) bool {
	for _, ws := range workspaces {
		expanded := ExpandPath(ws)
		if isSegmentPrefix(expanded, projectPath) {
			return true
		}
	}
	return false
}

func isSegmentPrefix(workspacePath, projectPath string) bool {
	wsSegs := strings.Split(strings.TrimPrefix(workspacePath, "/"), "/")
	projSegs := strings.Split(strings.TrimPrefix(projectPath, "/"), "/")

	if len(wsSegs) > len(projSegs) {
		return false
	}

	for i, seg := range wsSegs {
		if projSegs[i] != seg {
			return false
		}
	}
	return true
}
