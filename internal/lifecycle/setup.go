package lifecycle

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nextzhou/argus/internal/assets"
)

// Setup performs project-level Argus setup.
//
// It creates the .argus/ directory structure, releases built-in assets,
// and sets up Agent hook configurations. The operation is idempotent.
func Setup(projectRoot string, _ bool) error {
	_, err := SetupWithReport(projectRoot)
	return err
}

// SetupWithReport performs project-level setup and returns the
// summarized filesystem changes produced by the operation.
func SetupWithReport(projectRoot string) (ProjectOperationResult, error) {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return ProjectOperationResult{}, err
	}

	tracker := newMutationTracker()
	dirs := []string{
		"workflows",
		"invariants",
		"rules",
		"pipelines",
		"logs",
		"data",
		"tmp",
	}

	for _, dir := range dirs {
		path := filepath.Join(projectRoot, ".argus", dir)
		if err := ensureDirTracked(path, tracker); err != nil {
			return ProjectOperationResult{}, fmt.Errorf("creating %s: %w", path, err)
		}
	}

	releaseMap := map[string][]string{
		"workflows": {filepath.Join(".argus", "workflows")},
	}

	for srcDir, dstDirs := range releaseMap {
		for _, dstDir := range dstDirs {
			if err := releaseAssetsTracked(projectRoot, srcDir, dstDir, tracker); err != nil {
				return ProjectOperationResult{}, fmt.Errorf("releasing %s assets to %s: %w", srcDir, dstDir, err)
			}
		}
	}

	if err := releaseAssetFileTracked(projectRoot, filepath.Join("invariants", "argus-project-init.yaml"), filepath.Join(".argus", "invariants", "argus-project-init.yaml"), tracker); err != nil {
		return ProjectOperationResult{}, fmt.Errorf("releasing project invariant asset: %w", err)
	}

	for _, skillName := range ProjectSkillNames() {
		if err := releaseGlobalSkill(skillName, projectSkillRoots(projectRoot), tracker); err != nil {
			return ProjectOperationResult{}, fmt.Errorf("releasing project skill %s: %w", skillName, err)
		}
	}

	if err := setupGlobalSkills(homeDir, tracker); err != nil {
		return ProjectOperationResult{}, fmt.Errorf("refreshing global skills: %w", err)
	}
	if err := pruneManagedSkills(projectSkillRoots(projectRoot), ProjectSkillNames(), tracker); err != nil {
		return ProjectOperationResult{}, fmt.Errorf("pruning managed project skills: %w", err)
	}
	if err := pruneManagedYAMLFiles(filepath.Join(projectRoot, ".argus", "workflows"), projectBuiltinWorkflowIDs(), tracker); err != nil {
		return ProjectOperationResult{}, fmt.Errorf("pruning managed project workflows: %w", err)
	}
	if err := pruneManagedYAMLFiles(filepath.Join(projectRoot, ".argus", "invariants"), projectBuiltinInvariantIDs(), tracker); err != nil {
		return ProjectOperationResult{}, fmt.Errorf("pruning managed project invariants: %w", err)
	}

	if err := setupHooks(projectRoot, managedAgents(), tracker); err != nil {
		return ProjectOperationResult{}, fmt.Errorf("setting up hooks: %w", err)
	}

	return ProjectOperationResult{
		Root:   projectRoot,
		Report: buildProjectSetupReport(projectRoot, homeDir, tracker),
	}, nil
}

func releaseAssetFileTracked(projectRoot, srcPath, dstPath string, tracker *mutationTracker) error {
	data, err := assets.ReadAsset(srcPath)
	if err != nil {
		return fmt.Errorf("reading asset %s: %w", srcPath, err)
	}

	if err := writeFileTracked(filepath.Join(projectRoot, dstPath), data, tracker); err != nil {
		return fmt.Errorf("writing %s: %w", dstPath, err)
	}

	return nil
}

func managedAgents() []string {
	return []string{agentClaudeCode, agentCodex, agentOpenCode}
}

func releaseAssetsTracked(projectRoot, srcDir, dstDir string, tracker *mutationTracker) error {
	if err := assets.WalkAssets(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking %s assets: %w", srcDir, err)
		}

		if path == srcDir {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("computing %s relative path: %w", path, err)
		}

		dstPath := filepath.Join(projectRoot, dstDir, relPath)
		if d.IsDir() {
			if err := os.MkdirAll(dstPath, 0o700); err != nil {
				return fmt.Errorf("creating %s: %w", dstPath, err)
			}
			return nil
		}

		data, err := assets.ReadAsset(path)
		if err != nil {
			return fmt.Errorf("reading asset %s: %w", path, err)
		}

		if err := writeFileTracked(dstPath, data, tracker); err != nil {
			return fmt.Errorf("writing %s: %w", dstPath, err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("releasing assets from %s to %s: %w", srcDir, dstDir, err)
	}
	return nil
}

func ensureDirTracked(path string, tracker *mutationTracker) error {
	info, err := os.Stat(path)
	switch {
	case err == nil && info.IsDir():
		return nil
	case err == nil && !info.IsDir():
		return fmt.Errorf("%s already exists and is not a directory", path)
	case err != nil && !os.IsNotExist(err):
		return fmt.Errorf("stating %s: %w", path, err)
	}

	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("creating directory %s: %w", path, err)
	}

	tracker.recordCreated(path)
	return nil
}

// CheckSetupPreconditions validates that setup can proceed.
// It returns the setup target directory (the current working directory)
// and whether that directory is a subdirectory of the enclosing Git repository.
func CheckSetupPreconditions() (projectRoot string, isSubdir bool, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("getting working directory: %w", err)
	}

	projectRoot, err = filepath.Abs(cwd)
	if err != nil {
		return "", false, fmt.Errorf("resolving working directory: %w", err)
	}

	gitRoot, foundGit, err := findAncestorPath(projectRoot, ".git")
	if err != nil {
		return "", false, fmt.Errorf("finding git root: %w", err)
	}
	if !foundGit {
		return "", false, fmt.Errorf("argus requires a git repository; please run 'git init' first")
	}

	if ancestorArgus, foundArgus, err := findAncestorArgus(projectRoot); err != nil {
		return "", false, fmt.Errorf("checking ancestor .argus directories: %w", err)
	} else if foundArgus {
		return "", false, fmt.Errorf("ancestor .argus/ found at %s — nested project-level Argus setup is not supported", ancestorArgus)
	}

	return projectRoot, projectRoot != gitRoot, nil
}

func findAncestorArgus(start string) (string, bool, error) {
	parent := filepath.Dir(start)
	for parent != start {
		argusDir := filepath.Join(parent, ".argus")
		isDir, err := pathIsDir(argusDir)
		if err != nil {
			return "", false, err
		}
		if isDir {
			return parent, true, nil
		}

		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		start = parent
		parent = next
	}

	return "", false, nil
}

func findAncestorPath(start, name string) (string, bool, error) {
	current := start
	for {
		exists, err := pathExists(filepath.Join(current, name))
		if err != nil {
			return "", false, err
		}
		if exists {
			return current, true, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", false, nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stating %s: %w", path, err)
}

func pathIsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stating %s: %w", path, err)
}
