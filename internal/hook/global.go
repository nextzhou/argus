package hook

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nextzhou/argus/internal/workspace"
)

const globalTickInstallGuidance = `[Argus] This project is inside a registered workspace but Argus is not installed.

To install Argus in this project, run:
  argus install --yes

For guidance, use the argus-install Skill.
`

// HandleGlobalTick evaluates the global tick decision tree before project-level
// tick logic runs. Callers should invoke it only for global hook executions and
// use the returned guidance text, if any, instead of falling through to the
// project-level tick flow.
func HandleGlobalTick(cwd string, stdin io.Reader, agent string) (string, error) {
	input, err := ParseInput(stdin, agent)
	if err != nil {
		return "", nil
	}
	if IsSubAgent(input) {
		return "", nil
	}

	root, err := workspace.FindProjectRoot(cwd)
	if err != nil {
		return "", fmt.Errorf("finding project root: %w", err)
	}
	if root == nil {
		return "", nil
	}
	if root.HasArgus {
		return "", nil
	}

	configPath := globalTickConfigPath()
	if configPath == "" {
		return "", errors.New("determining workspace config path: home directory unavailable")
	}

	config, err := workspace.LoadConfig(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("loading workspace config: %w", err)
	}

	expandedWorkspaces := make([]string, 0, len(config.Workspaces))
	for _, workspacePath := range config.Workspaces {
		expandedWorkspaces = append(expandedWorkspaces, workspace.ExpandPath(workspacePath))
	}

	if !workspace.IsInWorkspace(root.Path, expandedWorkspaces) {
		return "", nil
	}

	return globalTickInstallGuidance, nil
}

func globalTickConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "argus", "config.yaml")
}
