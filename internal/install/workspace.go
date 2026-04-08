package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/nextzhou/argus/internal/assets"
	workspacecfg "github.com/nextzhou/argus/internal/workspace"
)

// InstallWorkspace registers a workspace path and installs global Argus resources.
//
//nolint:revive // package-qualified API name is required by the CLI/install surface.
func InstallWorkspace(path string) error {
	if _, err := validateWorkspacePath(path); err != nil {
		return err
	}

	normalizedPath, err := workspacecfg.NormalizePath(path)
	if err != nil {
		return fmt.Errorf("normalizing workspace path: %w", err)
	}

	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return err
	}

	configPath := userConfigPathForHome(homeDir)
	config, err := loadWorkspaceConfig(configPath)
	if err != nil {
		return err
	}

	if slices.Contains(config.Workspaces, normalizedPath) {
		if _, err := os.Stderr.WriteString("workspace already registered: " + normalizedPath + "\n"); err != nil {
			return fmt.Errorf("writing duplicate workspace warning: %w", err)
		}
		return nil
	}

	config.Workspaces = workspacecfg.DeduplicateWorkspaces(append(config.Workspaces, normalizedPath))
	if err := workspacecfg.SaveConfig(configPath, config); err != nil {
		return fmt.Errorf("saving workspace config: %w", err)
	}

	if err := InstallGlobalHooks(supportedAgents); err != nil {
		return fmt.Errorf("installing global hooks: %w", err)
	}

	if err := InstallGlobalSkills(); err != nil {
		return fmt.Errorf("installing global skills: %w", err)
	}

	return nil
}

// InstallGlobalHooks installs global Argus hook files for the requested agents.
//
//nolint:revive // package-qualified API name is required by the CLI/install surface.
func InstallGlobalHooks(agents []string) error {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return err
	}

	for _, agent := range agents {
		if err := installGlobalHooksForAgent(homeDir, agent); err != nil {
			return fmt.Errorf("installing %s global hooks: %w", agent, err)
		}
	}

	return nil
}

// InstallGlobalSkills releases independent Argus skills to global Agent skill directories.
//
//nolint:revive // package-qualified API name is required by the CLI/install surface.
func InstallGlobalSkills() error {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return err
	}

	targetRoots := globalSkillPathsForHome(homeDir)
	for _, skillName := range GlobalSkillNames() {
		if err := releaseGlobalSkill(skillName, targetRoots); err != nil {
			return fmt.Errorf("releasing global skill %s: %w", skillName, err)
		}
	}

	return nil
}

// GlobalSkillNames returns the built-in skills that are safe for global distribution.
func GlobalSkillNames() []string {
	return []string{"argus-install", "argus-uninstall", "argus-doctor"}
}

// GlobalSkillPaths returns the global Agent skill directories Argus manages.
func GlobalSkillPaths() []string {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return nil
	}

	return globalSkillPathsForHome(homeDir)
}

// UserConfigPath returns the absolute path of the user-level Argus config file.
func UserConfigPath() string {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return ""
	}

	return userConfigPathForHome(homeDir)
}

func validateWorkspacePath(path string) (string, error) {
	absolutePath, err := filepath.Abs(workspacecfg.ExpandPath(path))
	if err != nil {
		return "", fmt.Errorf("resolving workspace path: %w", err)
	}

	info, err := os.Stat(absolutePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("workspace path does not exist: %s", path)
		}
		return "", fmt.Errorf("stating workspace path %q: %w", absolutePath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace path is not a directory: %s", path)
	}

	return absolutePath, nil
}

func loadWorkspaceConfig(configPath string) (*workspacecfg.Config, error) {
	config, err := workspacecfg.LoadConfig(configPath)
	if err == nil {
		return config, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return &workspacecfg.Config{}, nil
	}

	return nil, fmt.Errorf("loading workspace config: %w", err)
}

func installGlobalHooksForAgent(homeDir, agent string) error {
	switch agent {
	case agentClaudeCode:
		return installClaudeCodeHooksAt(filepath.Join(homeDir, claudeSettingsRelativePath), true)
	case agentCodex:
		return installCodexHooksAt(filepath.Join(homeDir, codexHooksRelativePath), true)
	case agentOpenCode:
		return installOpenCodeHooksAt(globalOpenCodePluginPathForHome(homeDir), true)
	default:
		_, err := RenderHookTemplate(agent, true)
		return err
	}
}

func releaseGlobalSkill(skillName string, targetRoots []string) error {
	sourceDir := filepath.Join("skills", skillName)

	return assets.WalkAssets(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking %s assets: %w", sourceDir, err)
		}
		if path == sourceDir {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("computing %s relative path: %w", path, err)
		}

		if d.IsDir() {
			for _, targetRoot := range targetRoots {
				if err := os.MkdirAll(filepath.Join(targetRoot, skillName, relPath), 0o755); err != nil {
					return fmt.Errorf("creating skill directory: %w", err)
				}
			}
			return nil
		}

		data, err := assets.ReadAsset(path)
		if err != nil {
			return err
		}

		for _, targetRoot := range targetRoots {
			if err := writeFile(filepath.Join(targetRoot, skillName, relPath), data); err != nil {
				return err
			}
		}

		return nil
	})
}

func resolveUserHomeDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}

	return homeDir, nil
}

func globalSkillPathsForHome(homeDir string) []string {
	return []string{
		filepath.Join(homeDir, ".claude", "skills"),
		filepath.Join(homeDir, ".agents", "skills"),
		filepath.Join(homeDir, ".config", "opencode", "skills"),
	}
}

func userConfigPathForHome(homeDir string) string {
	return filepath.Join(homeDir, ".config", "argus", "config.yaml")
}

func globalOpenCodePluginPathForHome(homeDir string) string {
	return filepath.Join(homeDir, ".config", "opencode", "plugins", "argus.ts")
}
