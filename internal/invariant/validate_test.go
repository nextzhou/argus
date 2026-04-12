package invariant

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// alwaysFound is a workflowChecker that always returns true (workflow exists).
func alwaysFound(_ string) bool { return true }

// neverFound is a workflowChecker that always returns false (workflow missing).
func neverFound(_ string) bool { return false }

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
	require.NoError(t, err)
}

func TestInspectDirectory(t *testing.T) {
	t.Run("valid dir with one invariant", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "my-check.yaml", `
version: v0.1.0
id: my-check
order: 10
check:
  - shell: "test -f README.md"
prompt: "Create a README"
`)
		report, err := InspectDirectory(dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Len(t, report.Files, 1)
		fr := report.Files["my-check.yaml"]
		require.NotNil(t, fr)
		assert.True(t, fr.Valid)
		assert.Equal(t, "my-check", fr.ID)
		assert.Empty(t, fr.Errors)
	})

	t.Run("invalid YAML file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "bad.yaml", `
this is: [not valid yaml
`)
		report, err := InspectDirectory(dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["bad.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		assert.NotEmpty(t, fr.Errors)
	})

	t.Run("duplicate IDs across two files", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "a.yaml", `
version: v0.1.0
id: dup-check
order: 10
check:
  - shell: "test -f README.md"
prompt: "Fix it"
`)
		writeFile(t, dir, "b.yaml", `
version: v0.1.0
id: dup-check
order: 20
check:
  - shell: "test -f LICENSE"
prompt: "Fix it"
`)
		report, err := InspectDirectory(dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		assert.False(t, report.Files["a.yaml"].Valid)
		assert.False(t, report.Files["b.yaml"].Valid)

		hasMsg := func(fr *FileResult, substr string) bool {
			for _, fe := range fr.Errors {
				if assert.ObjectsAreEqual(true, contains(fe.Message, substr)) {
					return true
				}
			}
			return false
		}
		assert.True(t, hasMsg(report.Files["a.yaml"], "duplicate"))
		assert.True(t, hasMsg(report.Files["b.yaml"], "duplicate"))
	})

	t.Run("argus prefix used by user", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "reserved.yaml", `
version: v0.1.0
id: argus-custom
order: 10
check:
  - shell: "test -f README.md"
prompt: "Fix it"
`)
		report, err := InspectDirectory(dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["reserved.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		foundNs := false
		for _, fe := range fr.Errors {
			if contains(fe.Message, "argus-") {
				foundNs = true
			}
		}
		assert.True(t, foundNs, "expected namespace violation error")
	})

	t.Run("built-in argus prefix allowed by allowlist", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "argus-project-init.yaml", `
version: v0.1.0
id: argus-project-init
order: 10
check:
  - shell: "true"
workflow: argus-project-init
`)
		report, err := InspectDirectory(dir, alwaysFound, func(id string) bool { return id == "argus-project-init" })
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.True(t, report.Files["argus-project-init.yaml"].Valid)
	})

	t.Run("invariant filename must match id", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "wrong-name.yaml", `
version: v0.1.0
id: lint-clean
order: 10
check:
  - shell: "true"
prompt: "Fix it"
`)
		report, err := InspectDirectory(dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["wrong-name.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		found := false
		for _, fe := range fr.Errors {
			if contains(fe.Message, `expected "lint-clean.yaml"`) {
				found = true
			}
		}
		assert.True(t, found, "expected filename mismatch error")
	})

	t.Run("missing workflow reference", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "check.yaml", `
version: v0.1.0
id: my-check
order: 10
check:
  - shell: "test -f README.md"
workflow: nonexistent-workflow
`)
		report, err := InspectDirectory(dir, neverFound, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["check.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		foundWf := false
		for _, fe := range fr.Errors {
			if contains(fe.Message, "workflow") {
				foundWf = true
			}
		}
		assert.True(t, foundWf, "expected workflow reference error")
	})

	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		report, err := InspectDirectory(dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Empty(t, report.Files)
	})

	t.Run("non-existent dir", func(t *testing.T) {
		_, err := InspectDirectory("/nonexistent/path/to/dir", alwaysFound, nil)
		require.Error(t, err)
	})

	t.Run("non-yaml files ignored", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "readme.txt", "not a yaml file")
		writeFile(t, dir, "my-check.yaml", `
version: v0.1.0
id: my-check
order: 10
check:
  - shell: "test -f README.md"
prompt: "Fix it"
`)
		report, err := InspectDirectory(dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Len(t, report.Files, 1)
		_, exists := report.Files["readme.txt"]
		assert.False(t, exists)
	})

	t.Run("workflow checker only called when workflow set", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "my-check.yaml", `
version: v0.1.0
id: my-check
order: 10
check:
  - shell: "test -f README.md"
prompt: "Fix it"
`)
		// neverFound should not cause error since workflow is empty
		report, err := InspectDirectory(dir, neverFound, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
