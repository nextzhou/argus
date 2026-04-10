package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/session"
	"github.com/nextzhou/argus/internal/sessiontest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeWorkflowSnoozeCmd runs the workflow snooze command and captures stdout output.
func executeWorkflowSnoozeCmd(t *testing.T, store session.Store, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newWorkflowSnoozeCmdWithSessionStore(store), args...)
}

const snoozePipelineRunning = `version: v0.1.0
workflow_id: release
status: running
current_job: build
started_at: "20240101T000000Z"
jobs:
  build:
    started_at: "20240101T000000Z"
`

func TestWorkflowSnooze(t *testing.T) {
	tests := []struct {
		name         string
		requiresSess bool
		pipelines    map[string]string
		wantErr      bool
		checkJSON    func(t *testing.T, data map[string]any)
		checkSess    func(t *testing.T, store session.Store, sessionID string)
	}{
		{
			name:         "snooze active pipeline",
			requiresSess: true,
			pipelines: map[string]string{
				"release-20240101T000000Z": snoozePipelineRunning,
			},
			wantErr: false,
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "ok", data["status"])
				snoozed, ok := data["snoozed"].([]any)
				require.True(t, ok, "snoozed should be an array")
				require.Len(t, snoozed, 1)
				assert.Equal(t, "release-20240101T000000Z", snoozed[0])
			},
			checkSess: func(t *testing.T, store session.Store, sessionID string) {
				s, err := store.Load(sessionID)
				require.NoError(t, err)
				assert.Contains(t, s.SnoozedPipelines, "release-20240101T000000Z")
			},
		},
		{
			name:         "no active pipeline",
			requiresSess: true,
			pipelines:    nil,
			wantErr:      true,
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "error", data["status"])
				assert.NotEmpty(t, data["message"])
			},
		},
		{
			name:         "idempotent snooze",
			requiresSess: true,
			pipelines: map[string]string{
				"release-20240101T000000Z": snoozePipelineRunning,
			},
			wantErr: false,
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "ok", data["status"])
			},
			checkSess: func(t *testing.T, store session.Store, sessionID string) {
				s, err := store.Load(sessionID)
				require.NoError(t, err)
				count := 0
				for _, id := range s.SnoozedPipelines {
					if id == "release-20240101T000000Z" {
						count++
					}
				}
				assert.Equal(t, 1, count, "pipeline should appear exactly once")
			},
		},
		{
			name:    "missing session flag",
			wantErr: true,
		},
		{
			name:         "multi-running anomaly snooze-all",
			requiresSess: true,
			pipelines: map[string]string{
				"release-20240101T000000Z": snoozePipelineRunning,
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
				snoozed, ok := data["snoozed"].([]any)
				require.True(t, ok, "snoozed should be an array")
				require.Len(t, snoozed, 2)

				ids := make([]string, len(snoozed))
				for i, v := range snoozed {
					ids[i] = v.(string)
				}
				assert.Contains(t, ids, "release-20240101T000000Z")
				assert.Contains(t, ids, "deploy-20240102T000000Z")
			},
			checkSess: func(t *testing.T, store session.Store, sessionID string) {
				s, err := store.Load(sessionID)
				require.NoError(t, err)
				assert.Contains(t, s.SnoozedPipelines, "release-20240101T000000Z")
				assert.Contains(t, s.SnoozedPipelines, "deploy-20240102T000000Z")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			// Ensure .argus/ exists for scope resolution even when no pipelines are written.
			require.NoError(t, os.MkdirAll(filepath.Join(".argus", "pipelines"), 0o700))
			store := sessiontest.NewMemoryStore()
			sessionID := ""

			if tt.pipelines != nil {
				for instanceID, content := range tt.pipelines {
					writePipelineFixture(t, instanceID, content)
				}
			}

			args := []string(nil)
			if tt.requiresSess {
				sessionID = sessiontest.NewSessionID(t, "snooze")
				args = []string{"--session", sessionID}
			}

			output, cmdErr := executeWorkflowSnoozeCmd(t, store, args...)

			if tt.wantErr {
				require.Error(t, cmdErr)
			} else {
				require.NoError(t, cmdErr)
			}

			if tt.name == "missing session flag" {
				return
			}

			var data map[string]any
			require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))

			if tt.checkJSON != nil {
				tt.checkJSON(t, data)
			}
			if tt.checkSess != nil {
				tt.checkSess(t, store, sessionID)
			}
		})
	}
}

func TestWorkflowSnoozeIdempotent(t *testing.T) {
	t.Chdir(t.TempDir())
	store := sessiontest.NewMemoryStore()

	sessionID := sessiontest.NewSessionID(t, "snooze-idempotent")

	writePipelineFixture(t, "release-20240101T000000Z", snoozePipelineRunning)

	output1, err := executeWorkflowSnoozeCmd(t, store, "--session", sessionID)
	require.NoError(t, err)

	var data1 map[string]any
	require.NoError(t, json.Unmarshal(output1, &data1))
	assert.Equal(t, "ok", data1["status"])

	output2, err := executeWorkflowSnoozeCmd(t, store, "--session", sessionID)
	require.NoError(t, err)

	var data2 map[string]any
	require.NoError(t, json.Unmarshal(output2, &data2))
	assert.Equal(t, "ok", data2["status"])

	s, err := store.Load(sessionID)
	require.NoError(t, err)

	count := 0
	for _, id := range s.SnoozedPipelines {
		if id == "release-20240101T000000Z" {
			count++
		}
	}
	assert.Equal(t, 1, count, "pipeline should appear exactly once after double snooze")
}
