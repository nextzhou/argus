package core

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatSourceRef(t *testing.T) {
	t.Run("project file renders relative path only", func(t *testing.T) {
		projectRoot := filepath.Join(string(filepath.Separator), "repo")
		source := SourceRef{
			Kind: SourceFile,
			Raw:  filepath.Join(projectRoot, ".argus", "workflows", "build.yaml"),
		}

		assert.Equal(t, filepath.Join(".argus", "workflows", "build.yaml"), FormatSourceRef(projectRoot, source))
	})

	t.Run("outside project file renders home-collapsed absolute path", func(t *testing.T) {
		homeDir := filepath.Join(string(filepath.Separator), "Users", "nextzhou")
		t.Setenv("HOME", homeDir)

		source := SourceRef{
			Kind: SourceFile,
			Raw:  filepath.Join(homeDir, ".config", "argus", "logs", "hook.log"),
		}

		assert.Equal(t, filepath.Join("~", ".config", "argus", "logs", "hook.log"), FormatSourceRef(filepath.Join(homeDir, "repo"), source))
	})

	t.Run("embedded asset renders with prefix", func(t *testing.T) {
		source := SourceRef{
			Kind: SourceEmbeddedAsset,
			Raw:  "workflows/argus-project-init.yaml",
		}

		assert.Equal(t, "embedded asset: workflows/argus-project-init.yaml", FormatSourceRef("", source))
	})

	t.Run("synthetic source renders with prefix", func(t *testing.T) {
		source := SourceRef{
			Kind: SourceSynthetic,
			Raw:  "test-fixture:bad-yaml",
		}

		assert.Equal(t, "synthetic: test-fixture:bad-yaml", FormatSourceRef("", source))
	})
}
