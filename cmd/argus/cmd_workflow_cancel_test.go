package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeCancelCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := newWorkflowCancelCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	cmdErr := cmd.Execute()

	require.NoError(t, w.Close())
	os.Stdout = old

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return out, cmdErr
}

func TestWorkflowCancel(t *testing.T) {
	tests := []struct {
		name      string
		pipelines map[string]string
		wantErr   bool
		checkJSON func(t *testing.T, data map[string]any)
	}{
		{
			name:      "no active pipeline",
			pipelines: nil,
			wantErr:   true,
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "error", data["status"])
				assert.NotEmpty(t, data["message"])
			},
		},
		{
			name: "single running pipeline",
			pipelines: map[string]string{
				"release-20240101T000000Z": `version: v0.1.0
workflow_id: release
status: running
current_job: build
started_at: "20240101T000000Z"
jobs:
  build:
    started_at: "20240101T000000Z"
`,
			},
			wantErr: false,
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "ok", data["status"])
				cancelled, ok := data["cancelled"].([]any)
				require.True(t, ok, "cancelled should be an array")
				require.Len(t, cancelled, 1)
				assert.Equal(t, "release-20240101T000000Z", cancelled[0])
			},
		},
		{
			name: "multiple running pipelines (anomaly)",
			pipelines: map[string]string{
				"release-20240101T000000Z": `version: v0.1.0
workflow_id: release
status: running
current_job: build
started_at: "20240101T000000Z"
jobs:
  build:
    started_at: "20240101T000000Z"
`,
				"deploy-20240102T000000Z": `version: v0.1.0
workflow_id: deploy
status: running
current_job: step1
started_at: "20240102T000000Z"
jobs:
  step1:
    started_at: "20240102T000000Z"
`,
			},
			wantErr: false,
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "ok", data["status"])
				cancelled, ok := data["cancelled"].([]any)
				require.True(t, ok, "cancelled should be an array")
				require.Len(t, cancelled, 2)

				ids := make([]string, len(cancelled))
				for i, v := range cancelled {
					ids[i] = v.(string)
				}
				assert.Contains(t, ids, "release-20240101T000000Z")
				assert.Contains(t, ids, "deploy-20240102T000000Z")
			},
		},
		{
			name: "completed pipeline is not active",
			pipelines: map[string]string{
				"release-20240101T000000Z": `version: v0.1.0
workflow_id: release
status: completed
current_job: null
started_at: "20240101T000000Z"
ended_at: "20240101T010000Z"
jobs:
  build:
    started_at: "20240101T000000Z"
    ended_at: "20240101T010000Z"
`,
			},
			wantErr: true,
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "error", data["status"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if tt.pipelines != nil {
				for instanceID, content := range tt.pipelines {
					writePipelineFixture(t, instanceID, content)
				}
			}

			output, cmdErr := executeCancelCmd(t)

			if tt.wantErr {
				assert.Error(t, cmdErr)
			} else {
				require.NoError(t, cmdErr)
			}

			var data map[string]any
			require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))

			if tt.checkJSON != nil {
				tt.checkJSON(t, data)
			}
		})
	}
}

func TestWorkflowCancelSavesPipelineState(t *testing.T) {
	t.Chdir(t.TempDir())

	writePipelineFixture(t, "release-20240101T000000Z", `version: v0.1.0
workflow_id: release
status: running
current_job: build
started_at: "20240101T000000Z"
jobs:
  build:
    started_at: "20240101T000000Z"
`)

	output, err := executeCancelCmd(t)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data))
	assert.Equal(t, "ok", data["status"])

	saved, readErr := os.ReadFile(filepath.Join(".argus", "pipelines", "release-20240101T000000Z.yaml"))
	require.NoError(t, readErr)
	assert.Contains(t, string(saved), "status: cancelled")
	assert.Contains(t, string(saved), "ended_at:")
}
