package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeListCmd runs the workflow list command and captures stdout output.
func executeListCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newWorkflowListCmd(), args...)
}

func TestWorkflowList(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles map[string]string
		wantErr    bool
		wantStatus string
		checkJSON  func(t *testing.T, data map[string]any)
	}{
		{
			name:       "empty directory",
			setupFiles: nil,
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				workflows, ok := data["workflows"].([]any)
				require.True(t, ok, "workflows should be an array")
				assert.Empty(t, workflows)
			},
		},
		{
			name: "multiple workflows sorted by id",
			setupFiles: map[string]string{
				"release.yaml": `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: lint
    prompt: "Run lint"
  - id: build
    prompt: "Build"
`,
				"deploy.yaml": `version: v0.1.0
id: deploy
description: Deploy to production
jobs:
  - id: deploy_step
    prompt: "Deploy"
`,
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				workflows, ok := data["workflows"].([]any)
				require.True(t, ok, "workflows should be an array")
				require.Len(t, workflows, 2)

				wf0 := workflows[0].(map[string]any)
				assert.Equal(t, "deploy", wf0["id"])
				assert.Equal(t, "Deploy to production", wf0["description"])
				assert.Equal(t, float64(1), wf0["jobs"])

				wf1 := workflows[1].(map[string]any)
				assert.Equal(t, "release", wf1["id"])
				assert.Equal(t, "Release workflow", wf1["description"])
				assert.Equal(t, float64(2), wf1["jobs"])
			},
		},
		{
			name: "shared yaml excluded",
			setupFiles: map[string]string{
				"_shared.yaml": `lint:
  id: lint
  prompt: "Run lint"
`,
				"release.yaml": `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: build
    prompt: "Build"
`,
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				workflows, ok := data["workflows"].([]any)
				require.True(t, ok, "workflows should be an array")
				require.Len(t, workflows, 1)

				wf0 := workflows[0].(map[string]any)
				assert.Equal(t, "release", wf0["id"])
			},
		},
		{
			name:       "directory does not exist",
			setupFiles: nil,
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				workflows, ok := data["workflows"].([]any)
				require.True(t, ok, "workflows should be an array")
				assert.Empty(t, workflows)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if tt.setupFiles != nil {
				workflowsDir := filepath.Join(".argus", "workflows")
				require.NoError(t, os.MkdirAll(workflowsDir, 0o755))
				for name, content := range tt.setupFiles {
					require.NoError(t, os.WriteFile(
						filepath.Join(workflowsDir, name),
						[]byte(content), 0o644,
					))
				}
			}

			output, cmdErr := executeListCmd(t)

			if tt.wantErr {
				assert.Error(t, cmdErr)
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

func TestWorkflowListNonYAMLFilesIgnored(t *testing.T) {
	t.Chdir(t.TempDir())

	workflowsDir := filepath.Join(".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(workflowsDir, "README.md"),
		[]byte("# Workflows"), 0o644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(workflowsDir, "release.yaml"),
		[]byte(`version: v0.1.0
id: release
jobs:
  - id: build
    prompt: "Build"
`), 0o644,
	))

	output, err := executeListCmd(t)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data))
	assert.Equal(t, "ok", data["status"])

	workflows := data["workflows"].([]any)
	require.Len(t, workflows, 1)
	assert.Equal(t, "release", workflows[0].(map[string]any)["id"])
}
