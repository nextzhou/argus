package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeStartCmd runs the workflow start command and captures stdout output.
func executeStartCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newWorkflowStartCmd(), args...)
}

func writeWorkflowFixture(t *testing.T, id, yamlContent string) {
	t.Helper()
	workflowsDir := filepath.Join(".argus", "workflows")
	pipelinesDir := filepath.Join(".argus", "pipelines")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o700))
	require.NoError(t, os.MkdirAll(pipelinesDir, 0o700))
	if yamlContent != "" {
		require.NoError(t, os.WriteFile(
			filepath.Join(workflowsDir, id+".yaml"),
			[]byte(yamlContent), 0o600,
		))
	}
}

func TestWorkflowStart(t *testing.T) {
	tests := []struct {
		name         string
		workflowID   string
		workflowYAML string
		setupActive  bool
		wantErr      bool
		wantStatus   string
		checkJSON    func(t *testing.T, data map[string]any)
	}{
		{
			name:       "start workflow with skill",
			workflowID: "test-wf",
			workflowYAML: `version: v0.1.0
id: test-wf
description: Test workflow
jobs:
  - id: build
    prompt: "Build the project"
    skill: "argus-build"
`,
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "running", data["pipeline_status"])
				assert.Equal(t, "1/1", data["progress"])
				nextJob, ok := data["next_job"].(map[string]any)
				require.True(t, ok, "next_job should be an object")
				assert.Equal(t, "build", nextJob["id"])
				assert.Contains(t, nextJob["prompt"].(string), "Build the project")
				assert.Equal(t, "argus-build", nextJob["skill"])
			},
		},
		{
			name:       "skill is null when job has no skill",
			workflowID: "no-skill",
			workflowYAML: `version: v0.1.0
id: no-skill
jobs:
  - id: review
    prompt: "Review the code"
`,
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				nextJob, ok := data["next_job"].(map[string]any)
				require.True(t, ok)
				assert.Nil(t, nextJob["skill"], "skill should be null when not set")
			},
		},
		{
			name:       "progress shows correct total for multiple jobs",
			workflowID: "multi-job",
			workflowYAML: `version: v0.1.0
id: multi-job
jobs:
  - id: lint
    prompt: "Run lint"
  - id: test_code
    prompt: "Run tests"
  - id: build
    prompt: "Build"
`,
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "1/3", data["progress"])
				nextJob, ok := data["next_job"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "lint", nextJob["id"])
			},
		},
		{
			name:       "error when workflow file not found",
			workflowID: "nonexistent",
			wantErr:    true,
			wantStatus: "error",
		},
		{
			name:       "error when active pipeline exists",
			workflowID: "existing",
			workflowYAML: `version: v0.1.0
id: existing
jobs:
  - id: step1
    prompt: "Do step 1"
`,
			setupActive: true,
			wantErr:     true,
			wantStatus:  "error",
		},
		{
			name:       "error on invalid workflow id",
			workflowID: "../etc/passwd",
			wantErr:    true,
			wantStatus: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeWorkflowFixture(t, tt.workflowID, tt.workflowYAML)

			if tt.setupActive {
				pipelineYAML := `version: v0.1.0
workflow_id: existing
status: running
current_job: step1
started_at: "20240101T000000Z"
jobs:
  step1:
    started_at: "20240101T000000Z"
`
				path := filepath.Join(".argus", "pipelines", "existing-20240101T000000Z.yaml")
				require.NoError(t, os.WriteFile(path, []byte(pipelineYAML), 0o600))
			}

			output, cmdErr := executeStartCmd(t, tt.workflowID)

			if tt.wantErr {
				require.Error(t, cmdErr)
			} else {
				require.NoError(t, cmdErr)
			}

			var data map[string]any
			require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
			assert.Equal(t, tt.wantStatus, data["status"])

			if tt.checkJSON != nil {
				tt.checkJSON(t, data)
			}
		})
	}
}

func TestWorkflowStartDefaultText(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "md-test", `version: v0.1.0
id: md-test
description: Default text test
jobs:
  - id: lint
    prompt: "Run linting"
    skill: "argus-lint"
  - id: build
    prompt: "Build project"
`)

	stdout, stderr, err := executeTextCommand(t, newWorkflowStartCmd(), "md-test")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "Argus: Pipeline")
	assert.Contains(t, stdout, "started (1/2)")
	assert.Contains(t, stdout, "Current job: lint")
	assert.Contains(t, stdout, "Prompt: Run linting")
	assert.Contains(t, stdout, "Skill: argus-lint")
	assert.Contains(t, stdout, `argus job-done --message "execution summary"`)
}

func TestWorkflowStartDefaultTextNoSkill(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkflowFixture(t, "md-no-skill", `version: v0.1.0
id: md-no-skill
jobs:
  - id: review
    prompt: "Review code"
`)

	stdout, stderr, err := executeTextCommand(t, newWorkflowStartCmd(), "md-no-skill")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	assert.Contains(t, stdout, "Current job: review")
	assert.NotContains(t, stdout, "Skill:")
}

func TestWorkflowStartBuiltinProjectInitStartsWithBootstrapJob(t *testing.T) {
	t.Chdir(t.TempDir())

	data, err := assets.ReadAsset("workflows/argus-project-init.yaml")
	require.NoError(t, err)
	writeWorkflowFixture(t, "argus-project-init", string(data))

	output, cmdErr := executeStartCmd(t, "argus-project-init")
	require.NoError(t, cmdErr)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(output, &payload))
	assert.Equal(t, "ok", payload["status"])
	assert.Equal(t, "running", payload["pipeline_status"])
	assert.Equal(t, "1/6", payload["progress"])

	nextJob, ok := payload["next_job"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "bootstrap_argus", nextJob["id"])
	assert.Nil(t, nextJob["skill"])
}

func TestWorkflowStartResolvesSharedRefsThroughProvider(t *testing.T) {
	t.Chdir(t.TempDir())
	writeSharedFixture(t, `jobs:
  build_job:
    id: build_from_shared
    prompt: "Build from shared"
    skill: "argus-build"
`)
	writeWorkflowFixture(t, "shared-ref", `version: v0.1.0
id: shared-ref
jobs:
  - ref: build_job
`)

	output, cmdErr := executeStartCmd(t, "shared-ref")
	require.NoError(t, cmdErr)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(output, &payload))
	assert.Equal(t, "ok", payload["status"])
	assert.Equal(t, "running", payload["pipeline_status"])

	nextJob, ok := payload["next_job"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "build_job", nextJob["id"])
	assert.Equal(t, "Build from shared", nextJob["prompt"])
	assert.Equal(t, "argus-build", nextJob["skill"])
}

func writeSharedFixture(t *testing.T, yamlContent string) {
	t.Helper()
	workflowsDir := filepath.Join(".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(workflowsDir, "_shared.yaml"),
		[]byte(yamlContent), 0o600,
	))
}
