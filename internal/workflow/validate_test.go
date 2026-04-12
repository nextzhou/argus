package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
	require.NoError(t, err)
}

func entryByBase(report *InspectReport, base string) *InspectEntry {
	for i := range report.Entries {
		if filepath.Base(report.Entries[i].Source.Raw) == base {
			return &report.Entries[i]
		}
	}
	return nil
}

func findingWithMessage(entry *InspectEntry, substr string) *core.Finding {
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

func findingAtPath(entry *InspectEntry, path string) *core.Finding {
	if entry == nil {
		return nil
	}
	for i := range entry.Findings {
		if entry.Findings[i].FieldPath == path {
			return &entry.Findings[i]
		}
	}
	return nil
}

func TestInspectDirectory(t *testing.T) {
	t.Run("valid dir with one workflow and shared", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "_shared.yaml", `
jobs:
  lint:
    prompt: "Run linting"
  code_review:
    prompt: "Review code"
`)
		writeTestFile(t, dir, "release.yaml", `
version: v0.1.0
id: release
jobs:
  - prompt: "Build release artifacts"
  - ref: lint
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Len(t, report.Entries, 2)

		shared := entryByBase(report, "_shared.yaml")
		require.NotNil(t, shared)
		assert.True(t, shared.Valid)
		assert.NotNil(t, shared.Shared)
		assert.Nil(t, shared.Workflow)
		assert.ElementsMatch(t, []string{"lint", "code_review"}, shared.Shared.Jobs)

		wf := entryByBase(report, "release.yaml")
		require.NotNil(t, wf)
		assert.True(t, wf.Valid)
		assert.NotNil(t, wf.Workflow)
		assert.Nil(t, wf.Shared)
		assert.Equal(t, "release", wf.Workflow.ID)
		assert.Equal(t, 2, wf.Workflow.Jobs)
		assert.Empty(t, wf.Findings)
	})

	t.Run("YAML syntax error", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "bad.yaml", `
this is: [not valid yaml
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "bad.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotEmpty(t, entry.Findings)
	})

	t.Run("missing required field id", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "noid.yaml", `
version: v0.1.0
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "noid.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, findingWithMessage(entry, "ID"))
	})

	t.Run("unknown key", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "unknown.yaml", `
version: v0.1.0
id: my-workflow
unknown_field: "bad"
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "unknown.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, findingWithMessage(entry, "unknown_field"))
	})

	t.Run("incompatible version", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "oldver.yaml", `
version: v2.0.0
id: release
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "oldver.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, findingWithMessage(entry, "version"))
	})

	t.Run("invalid workflow ID format", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "badid.yaml", `
version: v0.1.0
id: MY-WORKFLOW
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "badid.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
	})

	t.Run("argus prefix used by user", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "reserved.yaml", `
version: v0.1.0
id: argus-custom
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "reserved.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, findingWithMessage(entry, "argus-"))
	})

	t.Run("built-in argus prefix allowed by allowlist", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "argus-project-init.yaml", `
version: v0.1.0
id: argus-project-init
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory("", dir, func(id string) bool { return id == "argus-project-init" })
		require.NoError(t, err)
		assert.True(t, report.Valid)
		entry := entryByBase(report, "argus-project-init.yaml")
		require.NotNil(t, entry)
		assert.True(t, entry.Valid)
	})

	t.Run("workflow filename must match id", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "wrong-name.yaml", `
version: v0.1.0
id: release
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "wrong-name.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, findingWithMessage(entry, `expected "release.yaml"`))
	})

	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Empty(t, report.Entries)
	})

	t.Run("non-existent dir", func(t *testing.T) {
		_, err := InspectDirectory("", "/nonexistent/path/to/dir", nil)
		require.Error(t, err)
	})

	t.Run("non-yaml files ignored", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "readme.txt", "not a yaml file")
		writeTestFile(t, dir, "my-workflow.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Len(t, report.Entries, 1)
		assert.Nil(t, entryByBase(report, "readme.txt"))
	})
}

func TestDuplicateIDs(t *testing.T) {
	t.Run("same ID in two files", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "a.yaml", `
version: v0.1.0
id: dup-workflow
jobs:
  - prompt: "do something"
`)
		writeTestFile(t, dir, "b.yaml", `
version: v0.1.0
id: dup-workflow
jobs:
  - prompt: "do something else"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		aEntry := entryByBase(report, "a.yaml")
		bEntry := entryByBase(report, "b.yaml")
		require.NotNil(t, aEntry)
		require.NotNil(t, bEntry)
		assert.False(t, aEntry.Valid)
		assert.False(t, bEntry.Valid)
		assert.NotNil(t, findingWithMessage(aEntry, "duplicate"))
		assert.NotNil(t, findingWithMessage(bEntry, "duplicate"))
	})

	t.Run("unique IDs across files", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "workflow-a.yaml", `
version: v0.1.0
id: workflow-a
jobs:
  - prompt: "do something"
`)
		writeTestFile(t, dir, "workflow-b.yaml", `
version: v0.1.0
id: workflow-b
jobs:
  - prompt: "do something else"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.True(t, entryByBase(report, "workflow-a.yaml").Valid)
		assert.True(t, entryByBase(report, "workflow-b.yaml").Valid)
	})
}

func TestSharedYaml(t *testing.T) {
	t.Run("shared loads with jobs list", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "_shared.yaml", `
jobs:
  lint:
    prompt: "Run linting"
  code_review:
    prompt: "Review code"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)

		entry := entryByBase(report, "_shared.yaml")
		require.NotNil(t, entry)
		assert.True(t, entry.Valid)
		assert.NotNil(t, entry.Shared)
		assert.Nil(t, entry.Workflow)
		assert.ElementsMatch(t, []string{"lint", "code_review"}, entry.Shared.Jobs)
	})

	t.Run("shared absent not in entries", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "my-workflow.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.Nil(t, entryByBase(report, "_shared.yaml"))
	})

	t.Run("shared with parse error", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "_shared.yaml", `
this is: [bad yaml
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "_shared.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotEmpty(t, entry.Findings)
		assert.Nil(t, entry.Shared)
		assert.Nil(t, entry.Workflow)
	})
}

func TestTemplateCheck(t *testing.T) {
	t.Run("valid template in prompt", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "my-workflow.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - prompt: "Hello {{ .Name }}, welcome"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		entry := entryByBase(report, "my-workflow.yaml")
		require.NotNil(t, entry)
		assert.True(t, entry.Valid)
	})

	t.Run("unclosed template action", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "wf.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - prompt: "Hello {{ .Missing"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "wf.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, findingAtPath(entry, "jobs[0].prompt"))
	})

	t.Run("invalid template syntax", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "wf.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - prompt: "Hello {{{{ invalid }}}}"
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "wf.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
	})

	t.Run("no prompt means no template check", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "_shared.yaml", `
jobs:
  lint:
    prompt: "Run linting"
`)
		writeTestFile(t, dir, "my-workflow.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - ref: lint
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
	})
}

func TestRefValidation(t *testing.T) {
	t.Run("ref exists in shared", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "_shared.yaml", `
jobs:
  lint:
    prompt: "Run linting"
`)
		writeTestFile(t, dir, "my-workflow.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - ref: lint
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.True(t, report.Valid)
	})

	t.Run("ref to nonexistent shared job", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "_shared.yaml", `
jobs:
  lint:
    prompt: "Run linting"
`)
		writeTestFile(t, dir, "wf.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - ref: nonexistent
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "wf.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, findingWithMessage(entry, "nonexistent"))
	})

	t.Run("ref used but no shared yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "wf.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - ref: lint
`)
		report, err := InspectDirectory("", dir, nil)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		entry := entryByBase(report, "wf.yaml")
		require.NotNil(t, entry)
		assert.False(t, entry.Valid)
		assert.NotNil(t, findingWithMessage(entry, "lint"))
	})
}
