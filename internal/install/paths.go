package install

import "path/filepath"

// SkillPaths returns the project-level skill directories Argus manages.
func SkillPaths() []string {
	return []string{
		filepath.Join(".agents", "skills"),
		filepath.Join(".claude", "skills"),
	}
}
