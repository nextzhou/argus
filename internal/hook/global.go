package hook

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nextzhou/argus/internal/workspace"
)

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

	configPath := globalTickConfigPath()
	if configPath == "" {
		if root.HasArgus {
			return "", nil
		}
		return "", errors.New("determining workspace config path: home directory unavailable")
	}

	config, err := workspace.LoadConfig(configPath)
	if err != nil {
		// Fail-open for installed projects or missing config files.
		if root.HasArgus || errors.Is(err, fs.ErrNotExist) {
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

	if root.HasArgus {
		return renderWorkspaceGuide(agent)
	}
	return renderInstallGuidance()
}

// renderInstallGuidance renders the install guidance template for projects in a
// workspace that do not have Argus installed. Returns empty string on render
// failure (fail-open).
func renderInstallGuidance() (string, error) {
	result, err := renderTemplate("prompts/global-tick-install.md.tmpl", nil)
	if err != nil {
		slog.Warn("rendering install guidance template", "error", err)
		return "", nil
	}
	return result, nil
}

// renderWorkspaceGuide renders the workspace guide template for installed
// projects inside a registered workspace. Returns empty string on render
// failure (fail-open).
func renderWorkspaceGuide(agent string) (string, error) {
	data := struct{ Agents []string }{Agents: []string{agent}}
	result, err := renderTemplate("prompts/workspace-guide.md.tmpl", data)
	if err != nil {
		slog.Warn("rendering workspace guide template", "error", err)
		return "", nil
	}
	return result, nil
}

func globalTickConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "argus", "config.yaml")
}
