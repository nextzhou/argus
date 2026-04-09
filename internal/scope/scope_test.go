package scope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProjectScope(t *testing.T) {
	projectRoot := t.TempDir()

	artifactScope := NewProjectScope(projectRoot)
	require.Implements(t, (*Scope)(nil), artifactScope)

	impl, ok := artifactScope.(*fsScope)
	require.True(t, ok)

	root := filepath.Join(projectRoot, ".argus")
	assert.Equal(t, root, impl.root)
	assert.Equal(t, projectRoot, artifactScope.ProjectRoot())
	assert.Equal(t, filepath.Join(root, "pipelines"), artifactScope.PipelinesDir())
	assert.Equal(t, filepath.Join(root, "workflows"), artifactScope.WorkflowsDir())
	assert.Equal(t, filepath.Join(root, "logs"), artifactScope.LogsDir())
}

func TestNewGlobalScope(t *testing.T) {
	globalRoot := t.TempDir()
	projectRoot := filepath.Join(t.TempDir(), "project")

	artifactScope := NewGlobalScope(globalRoot, projectRoot)
	require.Implements(t, (*Scope)(nil), artifactScope)

	impl, ok := artifactScope.(*fsScope)
	require.True(t, ok)

	safeID := core.ProjectPathToSafeID(projectRoot)
	assert.Equal(t, globalRoot, impl.root)
	assert.Equal(t, projectRoot, artifactScope.ProjectRoot())
	assert.Equal(t, filepath.Join(globalRoot, "pipelines", safeID), artifactScope.PipelinesDir())
	assert.Contains(t, artifactScope.PipelinesDir(), safeID)
	assert.Equal(t, filepath.Join(globalRoot, "workflows"), artifactScope.WorkflowsDir())
	assert.Equal(t, filepath.Join(globalRoot, "logs"), artifactScope.LogsDir())
}

func TestLoadInvariants(t *testing.T) {
	projectRoot := t.TempDir()
	artifactScope := NewProjectScope(projectRoot)
	invariantsDir := filepath.Join(projectRoot, ".argus", "invariants")

	writeScopeTestFile(t, filepath.Join(invariantsDir, "lint-clean.yaml"), `version: v0.1.0
id: lint-clean
description: "lint must stay green"
auto: always
check:
  - shell: "test -f .lint-passed"
prompt: "Run lint"
`)
	writeScopeTestFile(t, filepath.Join(invariantsDir, "broken.yaml"), "{{invalid yaml")
	writeScopeTestFile(t, filepath.Join(invariantsDir, "_ignored.yaml"), `version: v0.1.0
id: ignored
check:
  - shell: "true"
prompt: "ignored"
`)
	writeScopeTestFile(t, filepath.Join(invariantsDir, "notes.txt"), "not yaml")

	invariants, err := artifactScope.LoadInvariants()
	require.NoError(t, err)
	require.Len(t, invariants, 1)

	assert.Equal(t, "lint-clean", invariants[0].ID)
	assert.Equal(t, "lint must stay green", invariants[0].Description)
	assert.Equal(t, "always", invariants[0].Auto)
}

func TestScanActivePipelines(t *testing.T) {
	projectRoot := t.TempDir()
	artifactScope := NewProjectScope(projectRoot)

	runningJob := "run_tests"
	require.NoError(t, pipeline.SavePipeline(artifactScope.PipelinesDir(), "release-20240115T103000Z", &pipeline.Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "release",
		Status:     "running",
		CurrentJob: &runningJob,
		StartedAt:  "20240115T103000Z",
		Jobs:       map[string]*pipeline.JobData{},
	}))

	endedAt := "20240115T110000Z"
	require.NoError(t, pipeline.SavePipeline(artifactScope.PipelinesDir(), "build-20240115T100000Z", &pipeline.Pipeline{
		Version:    "v0.1.0",
		WorkflowID: "build",
		Status:     "completed",
		CurrentJob: nil,
		StartedAt:  "20240115T100000Z",
		EndedAt:    &endedAt,
		Jobs:       map[string]*pipeline.JobData{},
	}))

	actives, warnings, err := artifactScope.ScanActivePipelines()
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, actives, 1)
	assert.Equal(t, "release-20240115T103000Z", actives[0].InstanceID)
	assert.Equal(t, "running", actives[0].Pipeline.Status)
	assert.Equal(t, "release", actives[0].Pipeline.WorkflowID)
}

func TestLoadWorkflow(t *testing.T) {
	projectRoot := t.TempDir()
	artifactScope := NewProjectScope(projectRoot)
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

	wf, err := artifactScope.LoadWorkflow("release")
	require.NoError(t, err)
	require.Len(t, wf.Jobs, 2)

	assert.Equal(t, "release", wf.ID)
	assert.Equal(t, "lint", wf.Jobs[0].ID)
	assert.Equal(t, "Run lint checks", wf.Jobs[0].Prompt)
	assert.Equal(t, "argus-run-lint", wf.Jobs[0].Skill)
	assert.Equal(t, "lint", wf.Jobs[0].Ref)
	assert.Equal(t, "run_tests", wf.Jobs[1].ID)
	assert.Equal(t, "Run go test ./...", wf.Jobs[1].Prompt)
}

func TestLoadWorkflowSummaries(t *testing.T) {
	projectRoot := t.TempDir()
	artifactScope := NewProjectScope(projectRoot)
	workflowsDir := filepath.Join(projectRoot, ".argus", "workflows")

	writeScopeTestFile(t, filepath.Join(workflowsDir, "release.yaml"), `version: v0.1.0
id: release
description: "Release workflow"
jobs:
  - id: lint
    prompt: "Run lint"
  - id: test
    prompt: "Run tests"
`)
	writeScopeTestFile(t, filepath.Join(workflowsDir, "build.yaml"), `version: v0.1.0
id: build
description: "Build workflow"
jobs:
  - id: compile
    prompt: "Build the binary"
`)
	writeScopeTestFile(t, filepath.Join(workflowsDir, "broken.yaml"), "{{invalid yaml")
	writeScopeTestFile(t, filepath.Join(workflowsDir, "_shared.yaml"), `jobs:
  lint:
    prompt: "Run lint"
`)
	writeScopeTestFile(t, filepath.Join(workflowsDir, "README.txt"), "ignore")

	summaries, err := artifactScope.LoadWorkflowSummaries()
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	summaryByID := make(map[string]WorkflowSummary, len(summaries))
	for _, summary := range summaries {
		summaryByID[summary.ID] = summary
	}

	assert.Equal(t, WorkflowSummary{ID: "build", Description: "Build workflow", Jobs: 1}, summaryByID["build"])
	assert.Equal(t, WorkflowSummary{ID: "release", Description: "Release workflow", Jobs: 2}, summaryByID["release"])
}

func TestInterfaceSatisfaction(t *testing.T) {
	tests := []struct {
		name     string
		newScope func() Scope
	}{
		{
			name: "project scope",
			newScope: func() Scope {
				return NewProjectScope(t.TempDir())
			},
		},
		{
			name: "global scope",
			newScope: func() Scope {
				return NewGlobalScope(t.TempDir(), filepath.Join(t.TempDir(), "project"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifactScope := tt.newScope()
			require.Implements(t, (*Scope)(nil), artifactScope)
			_, ok := artifactScope.(*fsScope)
			assert.True(t, ok)
		})
	}
}

func writeScopeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
