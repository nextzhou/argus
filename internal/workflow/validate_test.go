package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
	require.NoError(t, err)
}

func hasError(fr *FileResult, substr string) bool {
	for _, fe := range fr.Errors {
		if strings.Contains(fe.Message, substr) {
			return true
		}
	}
	return false
}

func hasErrorAtPath(fr *FileResult, path string) bool {
	for _, fe := range fr.Errors {
		if fe.Path == path {
			return true
		}
	}
	return false
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
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Len(t, report.Files, 2)

		shared := report.Files["_shared.yaml"]
		require.NotNil(t, shared)
		assert.True(t, shared.Valid)
		assert.NotNil(t, shared.Shared)
		assert.Nil(t, shared.Workflow)
		assert.ElementsMatch(t, []string{"lint", "code_review"}, shared.Shared.Jobs)

		wf := report.Files["release.yaml"]
		require.NotNil(t, wf)
		assert.True(t, wf.Valid)
		assert.NotNil(t, wf.Workflow)
		assert.Nil(t, wf.Shared)
		assert.Equal(t, "release", wf.Workflow.ID)
		assert.Equal(t, 2, wf.Workflow.Jobs)
	})

	t.Run("YAML syntax error", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "bad.yaml", `
this is: [not valid yaml
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["bad.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		assert.NotEmpty(t, fr.Errors)
	})

	t.Run("missing required field id", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "noid.yaml", `
version: v0.1.0
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["noid.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		assert.True(t, hasError(fr, "ID"), "expected error about missing ID")
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
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["unknown.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		assert.True(t, hasError(fr, "unknown_field"), "expected error about unknown field")
	})

	t.Run("incompatible version", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "oldver.yaml", `
version: v2.0.0
id: release
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["oldver.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		assert.True(t, hasError(fr, "version"), "expected version error")
	})

	t.Run("invalid workflow ID format", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "badid.yaml", `
version: v0.1.0
id: MY-WORKFLOW
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["badid.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
	})

	t.Run("argus prefix used by user", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "reserved.yaml", `
version: v0.1.0
id: argus-custom
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["reserved.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		assert.True(t, hasError(fr, "argus-"), "expected namespace violation error")
	})

	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Empty(t, report.Files)
	})

	t.Run("non-existent dir", func(t *testing.T) {
		_, err := InspectDirectory("/nonexistent/path/to/dir")
		require.Error(t, err)
	})

	t.Run("non-yaml files ignored", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "readme.txt", "not a yaml file")
		writeTestFile(t, dir, "check.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.Len(t, report.Files, 1)
		_, exists := report.Files["readme.txt"]
		assert.False(t, exists)
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
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		assert.False(t, report.Files["a.yaml"].Valid)
		assert.False(t, report.Files["b.yaml"].Valid)
		assert.True(t, hasError(report.Files["a.yaml"], "duplicate"))
		assert.True(t, hasError(report.Files["b.yaml"], "duplicate"))
	})

	t.Run("unique IDs across files", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "a.yaml", `
version: v0.1.0
id: workflow-a
jobs:
  - prompt: "do something"
`)
		writeTestFile(t, dir, "b.yaml", `
version: v0.1.0
id: workflow-b
jobs:
  - prompt: "do something else"
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		assert.True(t, report.Files["a.yaml"].Valid)
		assert.True(t, report.Files["b.yaml"].Valid)
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
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.True(t, report.Valid)

		fr := report.Files["_shared.yaml"]
		require.NotNil(t, fr)
		assert.True(t, fr.Valid)
		assert.NotNil(t, fr.Shared)
		assert.Nil(t, fr.Workflow)
		assert.ElementsMatch(t, []string{"lint", "code_review"}, fr.Shared.Jobs)
	})

	t.Run("shared absent not in files map", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "wf.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - prompt: "do something"
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		_, exists := report.Files["_shared.yaml"]
		assert.False(t, exists)
	})

	t.Run("shared with parse error", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "_shared.yaml", `
this is: [bad yaml
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["_shared.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		assert.NotEmpty(t, fr.Errors)
		assert.Nil(t, fr.Shared)
		assert.Nil(t, fr.Workflow)
	})
}

func TestTemplateCheck(t *testing.T) {
	t.Run("valid template in prompt", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "wf.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - prompt: "Hello {{ .Name }}, welcome"
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.True(t, report.Valid)
		fr := report.Files["wf.yaml"]
		require.NotNil(t, fr)
		assert.True(t, fr.Valid)
	})

	t.Run("unclosed template action", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "wf.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - prompt: "Hello {{ .Missing"
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["wf.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		assert.True(t, hasErrorAtPath(fr, "jobs[0].prompt"), "expected error at path jobs[0].prompt")
	})

	t.Run("invalid template syntax", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "wf.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - prompt: "Hello {{{{ invalid }}}}"
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["wf.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
	})

	t.Run("no prompt means no template check", func(t *testing.T) {
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
  - ref: lint
`)
		report, err := InspectDirectory(dir)
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
		writeTestFile(t, dir, "wf.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - ref: lint
`)
		report, err := InspectDirectory(dir)
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
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["wf.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		assert.True(t, hasError(fr, "nonexistent"), "expected error about missing ref")
	})

	t.Run("ref used but no shared yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "wf.yaml", `
version: v0.1.0
id: my-workflow
jobs:
  - ref: lint
`)
		report, err := InspectDirectory(dir)
		require.NoError(t, err)
		assert.False(t, report.Valid)
		fr := report.Files["wf.yaml"]
		require.NotNil(t, fr)
		assert.False(t, fr.Valid)
		assert.True(t, hasError(fr, "lint"), "expected error about missing ref")
	})
}
