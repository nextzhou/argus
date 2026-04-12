package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeInvariantCheckCmd runs the invariant check command and captures stdout output.
func executeInvariantCheckCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newInvariantCheckCmd(), args...)
}

func writeInvariantFixture(t *testing.T, id, yamlContent string) {
	t.Helper()
	invariantsDir := filepath.Join(".argus", "invariants")
	require.NoError(t, os.MkdirAll(invariantsDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(invariantsDir, id+".yaml"),
		[]byte(yamlContent), 0o600,
	))
}

const successfulInvariantYAML = `version: v0.1.0
id: check-pass
order: 20
description: Always passes
auto: always
check:
  - shell: "true"
    description: "always true"
prompt: "Fix it"
`

const failingInvariant = `version: v0.1.0
id: check-fail
order: 10
description: Always fails
auto: never
check:
  - shell: "false"
    description: "always false"
workflow: fix-it
prompt: "Run the fix-it workflow"
`

const failingNoDescription = `version: v0.1.0
id: no-desc
order: 30
auto: session_start
check:
  - shell: "echo hello"
  - shell: "false"
    description: "second step"
prompt: "Fix things"
`

func TestInvariantCheck(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		setup      func(t *testing.T)
		wantErr    bool
		wantStatus string
		checkJSON  func(t *testing.T, data map[string]any)
	}{
		{
			name: "check all with mix of pass and fail",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "check-pass", successfulInvariantYAML)
				writeInvariantFixture(t, "check-fail", failingInvariant)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.InDelta(t, 1, data["passed"], 0)
				assert.InDelta(t, 1, data["failed"], 0)

				results, ok := data["results"].([]any)
				require.True(t, ok, "results should be an array")
				require.Len(t, results, 2)

				r0 := results[0].(map[string]any)
				assert.Equal(t, "check-fail", r0["id"])
				assert.InDelta(t, 10, r0["order"], 0)
				assert.Equal(t, "Always fails", r0["description"])
				assert.Equal(t, "failed", r0["status"])
				assert.Equal(t, "fix-it", r0["workflow"])
				assert.Equal(t, "Run the fix-it workflow", r0["prompt"])

				steps0, ok := r0["steps"].([]any)
				require.True(t, ok, "steps should be an array")
				require.Len(t, steps0, 1)
				step0 := steps0[0].(map[string]any)
				assert.Equal(t, "always false", step0["description"])
				assert.Equal(t, "fail", step0["status"])

				r1 := results[1].(map[string]any)
				assert.Equal(t, "check-pass", r1["id"])
				assert.InDelta(t, 20, r1["order"], 0)
				assert.Equal(t, "Always passes", r1["description"])
				assert.Equal(t, "passed", r1["status"])
				assert.Nil(t, r1["workflow"])
				assert.Nil(t, r1["prompt"])
			},
		},
		{
			name: "check single by id",
			args: []string{"check-pass"},
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "check-pass", successfulInvariantYAML)
				writeInvariantFixture(t, "check-fail", failingInvariant)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.InDelta(t, 1, data["passed"], 0)
				assert.InDelta(t, 0, data["failed"], 0)

				results, ok := data["results"].([]any)
				require.True(t, ok)
				require.Len(t, results, 1)

				r0 := results[0].(map[string]any)
				assert.Equal(t, "check-pass", r0["id"])
				assert.InDelta(t, 20, r0["order"], 0)
				assert.Equal(t, "passed", r0["status"])
			},
		},
		{
			name:       "nonexistent id returns error",
			args:       []string{"does-not-exist"},
			wantErr:    true,
			wantStatus: "error",
			checkJSON: func(t *testing.T, data map[string]any) {
				msg, ok := data["message"].(string)
				require.True(t, ok)
				assert.Contains(t, msg, "invariant not found")
			},
		},
		{
			name:       "missing invariants directory returns empty results",
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.InDelta(t, 0, data["passed"], 0)
				assert.InDelta(t, 0, data["failed"], 0)
				results, ok := data["results"].([]any)
				require.True(t, ok)
				assert.Empty(t, results)
			},
		},
		{
			name: "failed invariant shows workflow and prompt",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "check-fail", failingInvariant)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.InDelta(t, 1, data["failed"], 0)
				results := data["results"].([]any)
				r0 := results[0].(map[string]any)
				assert.Equal(t, "failed", r0["status"])
				assert.Equal(t, "fix-it", r0["workflow"])
				assert.Equal(t, "Run the fix-it workflow", r0["prompt"])
			},
		},
		{
			name: "description fallback to shell commands",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "no-desc", failingNoDescription)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				results := data["results"].([]any)
				r0 := results[0].(map[string]any)
				assert.Equal(t, "no-desc", r0["id"])
				assert.Equal(t, "echo hello; false", r0["description"])
			},
		},
		{
			name: "check all reports invalid invariants separately",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "check-pass", successfulInvariantYAML)
				writeInvariantFixture(t, "broken-order", `version: v0.1.0
id: broken-order
check:
  - shell: "true"
prompt: "Fix it"
`)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.InDelta(t, 1, data["passed"], 0)
				assert.InDelta(t, 0, data["failed"], 0)
				results := data["results"].([]any)
				require.Len(t, results, 1)
				invalid, ok := data["invalid_invariants"].([]any)
				require.True(t, ok)
				require.Len(t, invalid, 1)
				issue := invalid[0].(map[string]any)
				assert.Equal(t, "broken-order.yaml", issue["file"])
				assert.Equal(t, "order", issue["path"])
			},
		},
		{
			name: "check single invalid target returns details",
			args: []string{"broken-order"},
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "broken-order", `version: v0.1.0
id: broken-order
check:
  - shell: "true"
prompt: "Fix it"
`)
			},
			wantErr:    true,
			wantStatus: "error",
			checkJSON: func(t *testing.T, data map[string]any) {
				msg, ok := data["message"].(string)
				require.True(t, ok)
				assert.Contains(t, msg, "invalid")
				details, ok := data["details"].([]any)
				require.True(t, ok)
				require.Len(t, details, 1)
				d0 := details[0].(map[string]any)
				assert.Equal(t, "order", d0["path"])
			},
		},
		{
			name: "check single ignores unrelated invalid invariants",
			args: []string{"check-pass"},
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "check-pass", successfulInvariantYAML)
				writeInvariantFixture(t, "broken-order", `version: v0.1.0
id: broken-order
check:
  - shell: "true"
prompt: "Fix it"
`)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				results := data["results"].([]any)
				require.Len(t, results, 1)
				invalid, ok := data["invalid_invariants"].([]any)
				require.True(t, ok)
				require.Len(t, invalid, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			// Scope resolution requires .argus/ to exist
			require.NoError(t, os.MkdirAll(".argus", 0o700))

			if tt.setup != nil {
				tt.setup(t)
			}

			output, cmdErr := executeInvariantCheckCmd(t, tt.args...)

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
