package scope

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nextzhou/argus/internal/workspace"
)

// ResolveScopeForTick resolves the artifact scope for tick after the caller has
// already identified the project root and whether the hook came from a global
// setup.
func ResolveScopeForTick(root *workspace.ProjectRoot, global bool) (*Resolved, error) {
	if root == nil {
		return nil, nil
	}

	if !global {
		if root.HasArgus {
			return NewProjectScope(root.Path), nil
		}
		return nil, nil
	}

	if root.HasArgus {
		return NewProjectScope(root.Path), nil
	}

	return resolveWorkspaceGlobalScope(root.Path)
}

// ResolveScope resolves the effective scope for CLI commands by discovering the
// nearest project root from cwd and then applying project-first, workspace-
// fallback rules.
func ResolveScope(cwd string) (*Resolved, error) {
	root, err := workspace.FindProjectRoot(cwd)
	if err != nil {
		return nil, fmt.Errorf("finding project root: %w", err)
	}
	if root == nil {
		return nil, nil
	}
	if root.HasArgus {
		return NewProjectScope(root.Path), nil
	}

	return resolveWorkspaceGlobalScope(root.Path)
}

func resolveWorkspaceGlobalScope(projectRoot string) (*Resolved, error) {
	configPath := globalConfigPath()
	if configPath == "" {
		return nil, nil
	}

	config, err := workspace.LoadConfig(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("loading workspace config: %w", err)
	}

	expandedWorkspaces := expandWorkspacePaths(config.Workspaces)
	if !workspace.IsInWorkspace(projectRoot, expandedWorkspaces) {
		return nil, nil
	}

	root := globalRoot()
	if root == "" {
		return nil, nil
	}

	return NewGlobalScope(root, projectRoot), nil
}

func expandWorkspacePaths(workspaces []string) []string {
	expanded := make([]string, 0, len(workspaces))
	for _, workspacePath := range workspaces {
		expanded = append(expanded, workspace.ExpandPath(workspacePath))
	}
	return expanded
}

func globalConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "argus", "config.yaml")
}

func globalRoot() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "argus")
}
