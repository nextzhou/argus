package invariant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/core"
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

func invariantEntryByBase(report *InspectReport, base string) *InspectEntry {
	for i := range report.Entries {
		if filepath.Base(report.Entries[i].Source.Raw) == base {
			return &report.Entries[i]
		}
	}
	return nil
}

func invariantFindingWithMessage(entry *InspectEntry, substr string) *core.Finding {
	if entry == nil {
		return nil
	}
	for i := range entry.Findings {
		if strings.Contains(entry.Findings[i].Message, substr) {
			return &entry.Findings[i]
		}
	}
	return nil
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
		report, err := InspectDirectory("", dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Len(t, report.Entries, 1)
		entry := invariantEntryByBase(report, "my-check.yaml")
		require.NotNil(t, entry)
		assert.True(t, entry.Valid)
		assert.Equal(t, "my-check", entry.ID)
		assert.Empty(t, entry.Findings)
	})

	t.Run("invalid YAML file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "bad.yaml", `
this is: [not valid yaml
`)
		report, err := InspectDirectory("", dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := invariantEntryByBase(report, "bad.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotEmpty(t, entry.Findings)
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
		report, err := InspectDirectory("", dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		aEntry := invariantEntryByBase(report, "a.yaml")
		bEntry := invariantEntryByBase(report, "b.yaml")
		require.NotNil(t, aEntry)
		require.NotNil(t, bEntry)
		assert.False(t, aEntry.Valid)
		assert.False(t, bEntry.Valid)
		assert.NotNil(t, invariantFindingWithMessage(aEntry, "duplicate"))
		assert.NotNil(t, invariantFindingWithMessage(bEntry, "duplicate"))
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
		report, err := InspectDirectory("", dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := invariantEntryByBase(report, "reserved.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, invariantFindingWithMessage(entry, "argus-"))
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
		report, err := InspectDirectory("", dir, alwaysFound, func(id string) bool { return id == "argus-project-init" })
		require.NoError(t, err)
		assert.True(t, report.Valid)
		entry := invariantEntryByBase(report, "argus-project-init.yaml")
		require.NotNil(t, entry)
		assert.True(t, entry.Valid)
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
		report, err := InspectDirectory("", dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := invariantEntryByBase(report, "wrong-name.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, invariantFindingWithMessage(entry, `expected "lint-clean.yaml"`))
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
		report, err := InspectDirectory("", dir, neverFound, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := invariantEntryByBase(report, "check.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, invariantFindingWithMessage(entry, "workflow"))
	})

	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		report, err := InspectDirectory("", dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Empty(t, report.Entries)
	})

	t.Run("non-existent dir", func(t *testing.T) {
		_, err := InspectDirectory("", "/nonexistent/path/to/dir", alwaysFound, nil)
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
		report, err := InspectDirectory("", dir, alwaysFound, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Len(t, report.Entries, 1)
		assert.Nil(t, invariantEntryByBase(report, "readme.txt"))
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
		report, err := InspectDirectory("", dir, neverFound, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
	})
}
