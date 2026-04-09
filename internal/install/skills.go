package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/nextzhou/argus/internal/assets"
	"github.com/nextzhou/argus/internal/core"
)

// BuiltinSkillNames returns the current built-in Argus skill names embedded in the binary.
func BuiltinSkillNames() ([]string, error) {
	names, err := assets.ListAssets("skills")
	if err != nil {
		return nil, fmt.Errorf("listing built-in skills: %w", err)
	}

	return names, nil
}

func projectSkillRoots(projectRoot string) []string {
	roots := make([]string, 0, len(SkillPaths()))
	for _, skillPath := range SkillPaths() {
		roots = append(roots, filepath.Join(projectRoot, skillPath))
	}

	return roots
}

func pruneManagedSkills(skillRoots []string, keep []string, tracker *mutationTracker) error {
	for _, skillRoot := range skillRoots {
		entries, err := os.ReadDir(skillRoot)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("reading skill directory %s: %w", skillRoot, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() || !core.IsArgusReserved(entry.Name()) || slices.Contains(keep, entry.Name()) {
				continue
			}

			if err := removeAllIfExists(filepath.Join(skillRoot, entry.Name()), tracker); err != nil {
				return fmt.Errorf("removing skill %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}
