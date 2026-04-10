package install

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nextzhou/argus/internal/core"
)

// UninstallProject removes the project-scoped Argus installation and reports
// the summarized filesystem changes produced by the operation.
func UninstallProject(projectRoot string) (Report, error) {
	homeDir, err := resolveUserHomeDir()
	if err != nil {
		return Report{}, err
	}

	tracker := newMutationTracker()
	if err := removeAllIfExists(filepath.Join(projectRoot, ".argus"), tracker); err != nil {
		return Report{}, fmt.Errorf("removing .argus: %w", err)
	}

	for _, skillPath := range SkillPaths() {
		skillsDir := filepath.Join(projectRoot, skillPath)
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Report{}, fmt.Errorf("reading skill directory %s: %w", skillsDir, err)
		}

		for _, entry := range entries {
			if entry.IsDir() && core.IsArgusReserved(entry.Name()) {
				if err := removeAllIfExists(filepath.Join(skillsDir, entry.Name()), tracker); err != nil {
					return Report{}, fmt.Errorf("removing skill %s: %w", entry.Name(), err)
				}
			}
		}
	}

	if err := uninstallHooks(projectRoot, managedAgents(), tracker); err != nil {
		return Report{}, fmt.Errorf("uninstalling hooks: %w", err)
	}

	return buildProjectUninstallReport(projectRoot, homeDir, tracker), nil
}
