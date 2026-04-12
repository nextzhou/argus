package scope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/artifact"
	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProjectScope(t *testing.T) {
	projectRoot := t.TempDir()

	resolved := NewProjectScope(projectRoot)
	require.NotNil(t, resolved)
	assert.Equal(t, KindProject, resolved.Kind())
	assert.Equal(t, projectRoot, resolved.ProjectRoot())

	artifacts := resolved.Artifacts()
	require.NotNil(t, artifacts)
	assert.IsType(t, &artifact.Set{}, artifacts)
}

func TestNewGlobalScope(t *testing.T) {
	globalRoot := t.TempDir()
	projectRoot := filepath.Join(t.TempDir(), "project")

	resolved := NewGlobalScope(globalRoot, projectRoot)
	require.NotNil(t, resolved)
	assert.Equal(t, KindGlobal, resolved.Kind())
	assert.Equal(t, projectRoot, resolved.ProjectRoot())

	artifacts := resolved.Artifacts()
	require.NotNil(t, artifacts)
	assert.IsType(t, &artifact.Set{}, artifacts)
}

func TestArtifactsInvariantCatalog(t *testing.T) {
	projectRoot := t.TempDir()
	resolved := NewProjectScope(projectRoot)
	invariantsDir := filepath.Join(projectRoot, ".argus", "invariants")

	writeScopeTestFile(t, filepath.Join(invariantsDir, "lint-clean.yaml"), `version: v0.1.0
id: lint-clean
order: 20
description: "lint must stay green"
auto: always
check:
  - shell: "test -f .lint-passed"
prompt: "Run lint"
`)
	writeScopeTestFile(t, filepath.Join(invariantsDir, "broken.yaml"), "{{invalid yaml")
	writeScopeTestFile(t, filepath.Join(invariantsDir, "_ignored.yaml"), `version: v0.1.0
id: ignored
order: 30
check:
  - shell: "true"
prompt: "ignored"
`)
	writeScopeTestFile(t, filepath.Join(invariantsDir, "wrong-name.yaml"), `version: v0.1.0
id: other-check
order: 40
check:
  - shell: "true"
prompt: "ignored"
`)

	catalog, err := resolved.Artifacts().Invariants().Catalog(true)
	require.NoError(t, err)
	require.Len(t, catalog.Invariants, 1)
	require.Len(t, catalog.Issues, 2)

	assert.Equal(t, "lint-clean", catalog.Invariants[0].ID)
	assert.Equal(t, "broken.yaml", catalog.Issues[0].File)
	assert.Equal(t, "wrong-name.yaml", catalog.Issues[1].File)
}

func TestArtifactsPipelineStore(t *testing.T) {
	projectRoot := t.TempDir()
	resolved := NewProjectScope(projectRoot)

	runningJob := "run_tests"
	require.NoError(t, resolved.Artifacts().Pipelines().Save("release-20240115T103000Z", &pipeline.Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "release",
		Status:     "running",
		CurrentJob: &runningJob,
		StartedAt:  "20240115T103000Z",
		Jobs:       map[string]*pipeline.JobData{},
	}))

	endedAt := "20240115T110000Z"
	require.NoError(t, resolved.Artifacts().Pipelines().Save("build-20240115T100000Z", &pipeline.Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "build",
		Status:     "completed",
		CurrentJob: nil,
		StartedAt:  "20240115T100000Z",
		EndedAt:    &endedAt,
		Jobs:       map[string]*pipeline.JobData{},
	}))

	actives, warnings, err := resolved.Artifacts().Pipelines().ScanActive()
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, actives, 1)
	assert.Equal(t, "release-20240115T103000Z", actives[0].InstanceID)
	assert.Equal(t, "release", actives[0].Pipeline.WorkflowID)
}

func TestArtifactsWorkflowProvider(t *testing.T) {
	projectRoot := t.TempDir()
	resolved := NewProjectScope(projectRoot)
	workflowsDir := filepath.Join(projectRoot, ".argus", "workflows")

	writeScopeTestFile(t, filepath.Join(workflowsDir, "_shared.yaml"), `jobs:
  lint:
    prompt: "Run lint checks"
    skill: "argus-run-lint"
`)
	writeScopeTestFile(t, filepath.Join(workflowsDir, "release.yaml"), `version: v0.1.0
id: release
description: "Release workflow"
jobs:
  - ref: lint
  - id: run_tests
    prompt: "Run go test ./..."
`)

	wf, err := resolved.Artifacts().Workflows().Load("release")
	require.NoError(t, err)
	require.Len(t, wf.Jobs, 2)
	assert.Equal(t, "release", wf.ID)
	assert.Equal(t, "lint", wf.Jobs[0].ID)

	summaries, err := resolved.Artifacts().Workflows().Summaries()
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, artifact.WorkflowSummary{ID: "release", Description: "Release workflow", Jobs: 2}, summaries[0])
}

func TestGlobalScopePipelineNamespaceIsolation(t *testing.T) {
	homeDir := t.TempDir()
	projectRoot := filepath.Join(homeDir, "work", "argus")
	globalRoot := filepath.Join(homeDir, ".config", "argus")
	resolved := NewGlobalScope(globalRoot, projectRoot)

	runningJob := "run_tests"
	require.NoError(t, resolved.Artifacts().Pipelines().Save("release-20240115T103000Z", &pipeline.Pipeline{
		Version:    core.SchemaVersion,
		WorkflowID: "release",
		Status:     "running",
		CurrentJob: &runningJob,
		StartedAt:  "20240115T103000Z",
		Jobs:       map[string]*pipeline.JobData{},
	}))

	expectedFile := filepath.Join(globalRoot, "pipelines", core.ProjectPathToSafeID(projectRoot), "release-20240115T103000Z.yaml")
	info, err := os.Stat(expectedFile)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func writeScopeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
