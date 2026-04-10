package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nextzhou/argus/internal/core"
)

// TeardownProject removes the project-scoped Argus setup and reports
// the summarized filesystem changes produced by the operation.
func TeardownProject(projectRoot string) (Report, error) {
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

	if err := teardownHooks(projectRoot, managedAgents(), tracker); err != nil {
		return Report{}, fmt.Errorf("tearing down hooks: %w", err)
	}

	return buildProjectTeardownReport(projectRoot, homeDir, tracker), nil
}
