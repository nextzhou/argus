package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/nextzhou/argus/internal/assets"
	"github.com/nextzhou/argus/internal/core"
	workspacecfg "github.com/nextzhou/argus/internal/workspace"
	"gopkg.in/yaml.v3"
)

type workspaceInstallState struct {
	homeDir           string
	configPath        string
	normalizedPath    string
	config            *workspacecfg.Config
	alreadyRegistered bool
}

type workspaceUninstallState struct {
	homeDir        string
	configPath     string
	normalizedPath string
	config         *workspacecfg.Config
	index          int
	isLast         bool
}

// InstallWorkspace registers a workspace path and installs global Argus resources.
//
//nolint:revive // package-qualified API name is required by the CLI/install surface.
func InstallWorkspace(path string) error {
	_, err := InstallWorkspaceWithReport(path)
	return err
}

// InstallWorkspaceWithReport registers a workspace path and returns the
// summarized filesystem changes produced by the operation.
//
//nolint:revive // package-qualified report API mirrors the existing install surface.
func InstallWorkspaceWithReport(path string) (WorkspaceOperationResult, error) {
	state, err := prepareWorkspaceInstall(path)
	if err != nil {
		return WorkspaceOperationResult{}, err
	}

	tracker := newMutationTracker()
	if !state.alreadyRegistered {
		state.config.Workspaces = workspacecfg.DeduplicateWorkspaces(append(state.config.Workspaces, state.normalizedPath))
		if err := saveWorkspaceConfigTracked(state.configPath, state.config, tracker); err != nil {
			return WorkspaceOperationResult{}, fmt.Errorf("saving workspace config: %w", err)
		}
	}

	if err := installGlobalHooks(state.homeDir, managedAgents(), tracker); err != nil {
		return WorkspaceOperationResult{}, fmt.Errorf("installing global hooks: %w", err)
	}

	if err := installGlobalSkills(state.homeDir, tracker); err != nil {
		return WorkspaceOperationResult{}, fmt.Errorf("installing global skills: %w", err)
	}

	if err := installGlobalArtifacts(state.homeDir, tracker); err != nil {
		return WorkspaceOperationResult{}, fmt.Errorf("installing global artifacts: %w", err)
	}

	return WorkspaceOperationResult{
		Path:              state.normalizedPath,
		AlreadyRegistered: state.alreadyRegistered,
		Report:            buildWorkspaceInstallReport(state.homeDir, tracker),
	}, nil
}

// PrepareWorkspaceInstall validates workspace install inputs and reports
// whether the operation is already satisfied.
func PrepareWorkspaceInstall(path string) (WorkspaceInstallPreview, error) {
	state, err := prepareWorkspaceInstall(path)
	if err != nil {
		return WorkspaceInstallPreview{}, err
	}

	return WorkspaceInstallPreview{
		Path:              state.normalizedPath,
		AlreadyRegistered: state.alreadyRegistered,
	}, nil
}

// InstallGlobalHooks installs global Argus hook files for the requested agents.
//
//nolint:revive // package-qualified API name is required by the CLI/install surface.
func InstallGlobalHooks(agents []string) error {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return err
	}

	return installGlobalHooks(homeDir, agents, nil)
}

func installGlobalHooks(homeDir string, agents []string, tracker *mutationTracker) error {
	for _, agent := range agents {
		if err := installGlobalHooksForAgent(homeDir, agent, tracker); err != nil {
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

	return installGlobalSkills(homeDir, nil)
}

func installGlobalSkills(homeDir string, tracker *mutationTracker) error {
	targetRoots := globalSkillPathsForHome(homeDir)
	for _, skillName := range GlobalSkillNames() {
		if err := releaseGlobalSkill(skillName, targetRoots, tracker); err != nil {
			return fmt.Errorf("releasing global skill %s: %w", skillName, err)
		}
	}

	if err := pruneManagedSkills(targetRoots, GlobalSkillNames(), tracker); err != nil {
		return fmt.Errorf("pruning global skills: %w", err)
	}

	return nil
}

// UninstallWorkspace removes a workspace registration and cleans up global resources
// if no workspaces remain.
func UninstallWorkspace(path string) error {
	_, err := UninstallWorkspaceWithReport(path)
	return err
}

// UninstallWorkspaceWithReport removes a workspace registration and returns the
// summarized filesystem changes produced by the operation.
func UninstallWorkspaceWithReport(path string) (WorkspaceOperationResult, error) {
	state, err := prepareWorkspaceUninstall(path)
	if err != nil {
		return WorkspaceOperationResult{}, err
	}

	tracker := newMutationTracker()
	state.config.Workspaces = slices.Delete(state.config.Workspaces, state.index, state.index+1)
	if err := saveWorkspaceConfigTracked(state.configPath, state.config, tracker); err != nil {
		return WorkspaceOperationResult{}, fmt.Errorf("saving workspace config: %w", err)
	}

	removedGlobalResources := len(state.config.Workspaces) == 0
	if removedGlobalResources {
		if err := uninstallGlobalHooks(state.homeDir, managedAgents(), tracker); err != nil {
			return WorkspaceOperationResult{}, fmt.Errorf("uninstalling global hooks: %w", err)
		}
		if err := uninstallGlobalSkills(state.homeDir, tracker); err != nil {
			return WorkspaceOperationResult{}, fmt.Errorf("uninstalling global skills: %w", err)
		}
	}

	return WorkspaceOperationResult{
		Path:                  state.normalizedPath,
		RemovedGlobalResource: removedGlobalResources,
		Report:                buildWorkspaceUninstallReport(state.homeDir, tracker, removedGlobalResources),
	}, nil
}

// PrepareWorkspaceUninstall validates workspace uninstall inputs and reports
// whether removing the registration will also remove global resources.
func PrepareWorkspaceUninstall(path string) (WorkspaceUninstallPreview, error) {
	state, err := prepareWorkspaceUninstall(path)
	if err != nil {
		return WorkspaceUninstallPreview{}, err
	}

	return WorkspaceUninstallPreview{
		Path:   state.normalizedPath,
		IsLast: state.isLast,
	}, nil
}

// UninstallGlobalHooks removes global Argus hook files for the requested agents.
func UninstallGlobalHooks(agents []string) error {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return err
	}

	return uninstallGlobalHooks(homeDir, agents, nil)
}

func uninstallGlobalHooks(homeDir string, agents []string, tracker *mutationTracker) error {
	for _, agent := range agents {
		if err := uninstallGlobalHooksForAgent(homeDir, agent, tracker); err != nil {
			return fmt.Errorf("uninstalling %s global hooks: %w", agent, err)
		}
	}

	return nil
}

// UninstallGlobalSkills removes argus-* skill directories from all global Agent skill paths.
func UninstallGlobalSkills() error {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return err
	}

	return uninstallGlobalSkills(homeDir, nil)
}

func uninstallGlobalSkills(homeDir string, tracker *mutationTracker) error {
	for _, skillPath := range globalSkillPathsForHome(homeDir) {
		entries, err := os.ReadDir(skillPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("reading skill directory %s: %w", skillPath, err)
		}

		for _, entry := range entries {
			if entry.IsDir() && core.IsArgusReserved(entry.Name()) {
				if err := removeAllIfExists(filepath.Join(skillPath, entry.Name()), tracker); err != nil {
					return fmt.Errorf("removing skill %s: %w", entry.Name(), err)
				}
			}
		}
	}

	return nil
}

// GlobalSkillNames returns the built-in skills that are safe for global distribution.
func GlobalSkillNames() []string {
	return []string{"argus-configure-invariant", "argus-configure-workflow", "argus-doctor", "argus-install", "argus-intro", "argus-uninstall"}
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

func prepareWorkspaceInstall(path string) (workspaceInstallState, error) {
	if _, err := validateWorkspacePath(path); err != nil {
		return workspaceInstallState{}, err
	}

	normalizedPath, err := workspacecfg.NormalizePath(path)
	if err != nil {
		return workspaceInstallState{}, fmt.Errorf("normalizing workspace path: %w", err)
	}

	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return workspaceInstallState{}, err
	}

	configPath := userConfigPathForHome(homeDir)
	config, err := loadWorkspaceConfig(configPath)
	if err != nil {
		return workspaceInstallState{}, err
	}

	return workspaceInstallState{
		homeDir:           homeDir,
		configPath:        configPath,
		normalizedPath:    normalizedPath,
		config:            config,
		alreadyRegistered: slices.Contains(config.Workspaces, normalizedPath),
	}, nil
}

func prepareWorkspaceUninstall(path string) (workspaceUninstallState, error) {
	normalizedPath, err := workspacecfg.NormalizePath(path)
	if err != nil {
		return workspaceUninstallState{}, fmt.Errorf("normalizing workspace path: %w", err)
	}

	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return workspaceUninstallState{}, err
	}

	configPath := userConfigPathForHome(homeDir)
	config, err := loadWorkspaceConfig(configPath)
	if err != nil {
		return workspaceUninstallState{}, err
	}

	idx := slices.Index(config.Workspaces, normalizedPath)
	if idx < 0 {
		return workspaceUninstallState{}, fmt.Errorf("workspace %q is not registered", normalizedPath)
	}

	return workspaceUninstallState{
		homeDir:        homeDir,
		configPath:     configPath,
		normalizedPath: normalizedPath,
		config:         config,
		index:          idx,
		isLast:         len(config.Workspaces) == 1,
	}, nil
}

func validateWorkspacePath(path string) (string, error) {
	absolutePath, err := filepath.Abs(workspacecfg.ExpandPath(path))
	if err != nil {
		return "", fmt.Errorf("resolving workspace path: %w", err)
	}

	//nolint:gosec // os.Stat is the validation step for the user-supplied workspace path before Argus persists it.
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

func saveWorkspaceConfigTracked(path string, config *workspacecfg.Config, tracker *mutationTracker) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := writeFileTracked(path, data, tracker); err != nil {
		return fmt.Errorf("writing config file %q: %w", path, err)
	}

	return nil
}

func installGlobalHooksForAgent(homeDir, agent string, tracker *mutationTracker) error {
	switch agent {
	case agentClaudeCode:
		return installClaudeCodeHooksAt(filepath.Join(homeDir, claudeSettingsRelativePath), true, tracker)
	case agentCodex:
		return installCodexHooksAt(filepath.Join(homeDir, codexHooksRelativePath), true, tracker)
	case agentOpenCode:
		return installOpenCodeHooksAt(globalOpenCodePluginPathForHome(homeDir), true, tracker)
	default:
		_, err := RenderHookTemplate(agent, true)
		return err
	}
}

func uninstallGlobalHooksForAgent(homeDir, agent string, tracker *mutationTracker) error {
	switch agent {
	case agentClaudeCode:
		return uninstallClaudeCodeHooksAt(filepath.Join(homeDir, claudeSettingsRelativePath), tracker)
	case agentCodex:
		return removeIfExistsTracked(filepath.Join(homeDir, codexHooksRelativePath), tracker)
	case agentOpenCode:
		return removeIfExistsTracked(globalOpenCodePluginPathForHome(homeDir), tracker)
	default:
		return nil
	}
}

func releaseGlobalSkill(skillName string, targetRoots []string, tracker *mutationTracker) error {
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
				if err := os.MkdirAll(filepath.Join(targetRoot, skillName, relPath), 0o700); err != nil {
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
			if err := writeFileTracked(filepath.Join(targetRoot, skillName, relPath), data, tracker); err != nil {
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

func installGlobalArtifacts(homeDir string, tracker *mutationTracker) error {
	globalRoot := filepath.Join(homeDir, ".config", "argus")

	// Create global directory structure.
	globalDirs := []string{"invariants", "workflows", "pipelines", "logs"}
	for _, dir := range globalDirs {
		if err := ensureDirTracked(filepath.Join(globalRoot, dir), tracker); err != nil {
			return fmt.Errorf("creating global %s directory: %w", dir, err)
		}
	}

	// Release only the global-specific invariant (argus-project-init).
	// Do NOT release project-level invariants (argus-init) to global scope
	// because their remediation workflows don't exist globally.
	data, err := assets.ReadAsset("invariants/argus-project-init.yaml")
	if err != nil {
		return fmt.Errorf("reading global invariant asset: %w", err)
	}
	dstPath := filepath.Join(globalRoot, "invariants", "argus-project-init.yaml")
	if err := writeFileTracked(dstPath, data, tracker); err != nil {
		return fmt.Errorf("writing global invariant: %w", err)
	}

	return nil
}
