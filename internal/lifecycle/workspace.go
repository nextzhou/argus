package lifecycle

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

type workspaceSetupState struct {
	homeDir           string
	configPath        string
	normalizedPath    string
	config            *workspacecfg.Config
	alreadyRegistered bool
}

type workspaceTeardownState struct {
	homeDir        string
	configPath     string
	normalizedPath string
	config         *workspacecfg.Config
	index          int
	isLast         bool
}

type workspaceTeardownOps struct {
	saveConfig        func(string, *workspacecfg.Config, *mutationTracker) error
	teardownHooks     func(string, []string, *mutationTracker) error
	teardownSkills    func(string, *mutationTracker) error
	teardownArtifacts func(string, *mutationTracker) error
}

func (ops workspaceTeardownOps) withDefaults() workspaceTeardownOps {
	if ops.saveConfig == nil {
		ops.saveConfig = saveWorkspaceConfigTracked
	}
	if ops.teardownHooks == nil {
		ops.teardownHooks = teardownGlobalHooks
	}
	if ops.teardownSkills == nil {
		ops.teardownSkills = teardownGlobalSkills
	}
	if ops.teardownArtifacts == nil {
		ops.teardownArtifacts = teardownGlobalArtifacts
	}
	return ops
}

// SetupWorkspace registers a workspace path and sets up global Argus resources.
func SetupWorkspace(path string) error {
	_, err := SetupWorkspaceWithReport(path)
	return err
}

// SetupWorkspaceWithReport registers a workspace path, refreshes global
// resources as needed, and returns the summarized filesystem changes produced
// by the operation.
func SetupWorkspaceWithReport(path string) (WorkspaceOperationResult, error) {
	state, err := prepareWorkspaceSetup(path)
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

	if err := setupGlobalHooks(state.homeDir, managedAgents(), tracker); err != nil {
		return WorkspaceOperationResult{}, fmt.Errorf("setting up global hooks: %w", err)
	}

	if err := setupGlobalSkills(state.homeDir, tracker); err != nil {
		return WorkspaceOperationResult{}, fmt.Errorf("setting up global skills: %w", err)
	}

	if err := setupGlobalArtifacts(state.homeDir, tracker); err != nil {
		return WorkspaceOperationResult{}, fmt.Errorf("setting up global artifacts: %w", err)
	}
	if err := pruneManagedYAMLFiles(filepath.Join(state.homeDir, ".config", "argus", "workflows"), stringSet(), tracker); err != nil {
		return WorkspaceOperationResult{}, fmt.Errorf("pruning managed global workflows: %w", err)
	}
	if err := pruneManagedYAMLFiles(filepath.Join(state.homeDir, ".config", "argus", "invariants"), globalBuiltinInvariantIDs(), tracker); err != nil {
		return WorkspaceOperationResult{}, fmt.Errorf("pruning managed global invariants: %w", err)
	}

	return WorkspaceOperationResult{
		Path:              state.normalizedPath,
		AlreadyRegistered: state.alreadyRegistered,
		Report:            buildWorkspaceSetupReport(state.homeDir, tracker),
	}, nil
}

// PrepareWorkspaceSetup validates workspace setup inputs and reports
// whether the operation is already satisfied.
func PrepareWorkspaceSetup(path string) (WorkspaceSetupPreview, error) {
	state, err := prepareWorkspaceSetup(path)
	if err != nil {
		return WorkspaceSetupPreview{}, err
	}

	return WorkspaceSetupPreview{
		Path:              state.normalizedPath,
		AlreadyRegistered: state.alreadyRegistered,
	}, nil
}

// SetupGlobalHooks sets up global Argus hook files for the requested agents.
func SetupGlobalHooks(agents []string) error {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return err
	}

	return setupGlobalHooks(homeDir, agents, nil)
}

func setupGlobalHooks(homeDir string, agents []string, tracker *mutationTracker) error {
	for _, agent := range agents {
		if err := setupGlobalHooksForAgent(homeDir, agent, tracker); err != nil {
			return fmt.Errorf("setting up %s global hooks: %w", agent, err)
		}
	}

	return nil
}

// SetupGlobalSkills releases independent Argus skills to global Agent skill directories.
func SetupGlobalSkills() error {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return err
	}

	return setupGlobalSkills(homeDir, nil)
}

func setupGlobalSkills(homeDir string, tracker *mutationTracker) error {
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

// TeardownWorkspace removes a workspace registration and tears down global resources
// if no workspaces remain.
func TeardownWorkspace(path string) error {
	_, err := TeardownWorkspaceWithReport(path)
	return err
}

// TeardownWorkspaceWithReport removes a workspace registration and returns the
// summarized filesystem changes produced by the operation.
func TeardownWorkspaceWithReport(path string) (WorkspaceOperationResult, error) {
	return teardownWorkspaceWithReport(path, workspaceTeardownOps{})
}

func teardownWorkspaceWithReport(path string, ops workspaceTeardownOps) (WorkspaceOperationResult, error) {
	ops = ops.withDefaults()

	state, err := prepareWorkspaceTeardown(path)
	if err != nil {
		return WorkspaceOperationResult{}, err
	}

	tracker := newMutationTracker()
	toreDownGlobalResources := state.isLast
	if toreDownGlobalResources {
		if err := ops.teardownHooks(state.homeDir, managedAgents(), tracker); err != nil {
			return WorkspaceOperationResult{}, fmt.Errorf("tearing down global hooks: %w", err)
		}
		if err := ops.teardownSkills(state.homeDir, tracker); err != nil {
			return WorkspaceOperationResult{}, fmt.Errorf("tearing down global skills: %w", err)
		}
		if err := ops.teardownArtifacts(state.homeDir, tracker); err != nil {
			return WorkspaceOperationResult{}, fmt.Errorf("tearing down global artifacts: %w", err)
		}
	} else {
		state.config.Workspaces = slices.Delete(state.config.Workspaces, state.index, state.index+1)
		if err := ops.saveConfig(state.configPath, state.config, tracker); err != nil {
			return WorkspaceOperationResult{}, fmt.Errorf("saving workspace config: %w", err)
		}
	}

	return WorkspaceOperationResult{
		Path:                    state.normalizedPath,
		ToreDownGlobalResources: toreDownGlobalResources,
		Report:                  buildWorkspaceTeardownReport(state.homeDir, tracker, toreDownGlobalResources),
	}, nil
}

// PrepareWorkspaceTeardown validates workspace teardown inputs and reports
// whether removing the registration will also remove global resources.
func PrepareWorkspaceTeardown(path string) (WorkspaceTeardownPreview, error) {
	state, err := prepareWorkspaceTeardown(path)
	if err != nil {
		return WorkspaceTeardownPreview{}, err
	}

	return WorkspaceTeardownPreview{
		Path:   state.normalizedPath,
		IsLast: state.isLast,
	}, nil
}

// TeardownGlobalHooks removes global Argus hook files for the requested agents.
func TeardownGlobalHooks(agents []string) error {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return err
	}

	return teardownGlobalHooks(homeDir, agents, nil)
}

func teardownGlobalHooks(homeDir string, agents []string, tracker *mutationTracker) error {
	for _, agent := range agents {
		if err := teardownGlobalHooksForAgent(homeDir, agent, tracker); err != nil {
			return fmt.Errorf("tearing down %s global hooks: %w", agent, err)
		}
	}

	return nil
}

// TeardownGlobalSkills removes argus-* skill directories from all global Agent skill paths.
func TeardownGlobalSkills() error {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return err
	}

	return teardownGlobalSkills(homeDir, nil)
}

func teardownGlobalSkills(homeDir string, tracker *mutationTracker) error {
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
	return []string{"argus-configure-invariant", "argus-configure-workflow", "argus-doctor", "argus-setup", "argus-intro", "argus-teardown"}
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

func prepareWorkspaceSetup(path string) (workspaceSetupState, error) {
	if _, err := validateWorkspacePath(path); err != nil {
		return workspaceSetupState{}, err
	}

	normalizedPath, err := workspacecfg.NormalizePath(path)
	if err != nil {
		return workspaceSetupState{}, fmt.Errorf("normalizing workspace path: %w", err)
	}

	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return workspaceSetupState{}, err
	}

	configPath := userConfigPathForHome(homeDir)
	config, err := loadWorkspaceConfig(configPath)
	if err != nil {
		return workspaceSetupState{}, err
	}

	return workspaceSetupState{
		homeDir:           homeDir,
		configPath:        configPath,
		normalizedPath:    normalizedPath,
		config:            config,
		alreadyRegistered: slices.Contains(config.Workspaces, normalizedPath),
	}, nil
}

func prepareWorkspaceTeardown(path string) (workspaceTeardownState, error) {
	normalizedPath, err := workspacecfg.NormalizePath(path)
	if err != nil {
		return workspaceTeardownState{}, fmt.Errorf("normalizing workspace path: %w", err)
	}

	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return workspaceTeardownState{}, err
	}

	configPath := userConfigPathForHome(homeDir)
	config, err := loadWorkspaceConfig(configPath)
	if err != nil {
		return workspaceTeardownState{}, err
	}

	idx := slices.Index(config.Workspaces, normalizedPath)
	if idx < 0 {
		return workspaceTeardownState{}, fmt.Errorf("workspace %q is not registered", normalizedPath)
	}

	return workspaceTeardownState{
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

func setupGlobalHooksForAgent(homeDir, agent string, tracker *mutationTracker) error {
	switch agent {
	case agentClaudeCode:
		return setupClaudeCodeHooksAt(filepath.Join(homeDir, claudeSettingsRelativePath), true, tracker)
	case agentCodex:
		return setupCodexHooksAt(filepath.Join(homeDir, codexHooksRelativePath), true, tracker)
	case agentOpenCode:
		return setupOpenCodeHooksAt(globalOpenCodePluginPathForHome(homeDir), true, tracker)
	default:
		_, err := RenderHookTemplate(agent, true)
		return err
	}
}

func teardownGlobalHooksForAgent(homeDir, agent string, tracker *mutationTracker) error {
	switch agent {
	case agentClaudeCode:
		return teardownClaudeCodeHooksAt(filepath.Join(homeDir, claudeSettingsRelativePath), tracker)
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

func setupGlobalArtifacts(homeDir string, tracker *mutationTracker) error {
	globalRoot := filepath.Join(homeDir, ".config", "argus")

	// Create the global directory structure.
	globalDirs := []string{"invariants", "workflows", "pipelines", "logs"}
	for _, dir := range globalDirs {
		if err := ensureDirTracked(filepath.Join(globalRoot, dir), tracker); err != nil {
			return fmt.Errorf("creating global %s directory: %w", dir, err)
		}
	}

	// Release only the global-specific invariant (argus-project-setup).
	// Do NOT release project-level invariants (argus-project-init) to global scope
	// because their remediation workflows don't exist globally.
	data, err := assets.ReadAsset("invariants/argus-project-setup.yaml")
	if err != nil {
		return fmt.Errorf("reading global invariant asset: %w", err)
	}
	dstPath := filepath.Join(globalRoot, "invariants", "argus-project-setup.yaml")
	if err := writeFileTracked(dstPath, data, tracker); err != nil {
		return fmt.Errorf("writing global invariant: %w", err)
	}

	return nil
}

func teardownGlobalArtifacts(homeDir string, tracker *mutationTracker) error {
	return removeAllIfExists(filepath.Join(homeDir, ".config", "argus"), tracker)
}
