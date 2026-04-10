package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

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

func pruneManagedYAMLFiles(dir string, keepIDs map[string]struct{}, tracker *mutationTracker) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading managed yaml directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".yaml")
		if !core.IsArgusReserved(id) {
			continue
		}
		if _, ok := keepIDs[id]; ok {
			continue
		}

		if err := removeIfExistsTracked(filepath.Join(dir, entry.Name()), tracker); err != nil {
			return fmt.Errorf("removing managed yaml %s: %w", entry.Name(), err)
		}
	}

	return nil
}

func projectBuiltinWorkflowIDs() map[string]struct{} {
	return stringSet("argus-project-init")
}

func projectBuiltinInvariantIDs() map[string]struct{} {
	return stringSet("argus-project-init")
}

func globalBuiltinInvariantIDs() map[string]struct{} {
	return stringSet("argus-project-setup")
}

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
