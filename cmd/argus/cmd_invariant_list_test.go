package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeInvariantListCmd runs the invariant list command and captures stdout output.
func executeInvariantListCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newInvariantListCmd(), args...)
}

func TestInvariantList(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T)
		wantStatus string
		checkJSON  func(t *testing.T, data map[string]any)
	}{
		{
			name:       "empty directory returns empty list",
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				invariants := mustJSONArray(t, data["invariants"])
				assert.Empty(t, invariants)
			},
		},
		{
			name: "list all invariants sorted by id",
			setup: func(t *testing.T) {
				t.Helper()
				writeInvariantFixture(t, "check-pass", successfulInvariantYAML)
				writeInvariantFixture(t, "check-fail", failingInvariant)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				invariants := mustJSONArray(t, data["invariants"])
				require.Len(t, invariants, 2)

				inv0 := mustJSONObject(t, invariants[0])
				assert.Equal(t, "check-fail", inv0["id"])
				assert.InDelta(t, 10, inv0["order"], 0)
				assert.Equal(t, "Always fails", inv0["description"])
				assert.Equal(t, "never", inv0["auto"])
				assert.InDelta(t, 1, inv0["checks"], 0)

				inv1 := mustJSONObject(t, invariants[1])
				assert.Equal(t, "check-pass", inv1["id"])
				assert.InDelta(t, 20, inv1["order"], 0)
				assert.Equal(t, "Always passes", inv1["description"])
				assert.Equal(t, "always", inv1["auto"])
				assert.InDelta(t, 1, inv1["checks"], 0)
			},
		},
		{
			name: "description fallback to shell commands",
			setup: func(t *testing.T) {
				t.Helper()
				writeInvariantFixture(t, "no-desc", failingNoDescription)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				invariants := mustJSONArray(t, data["invariants"])
				require.Len(t, invariants, 1)
				inv0 := mustJSONObject(t, invariants[0])
				assert.Equal(t, "no-desc", inv0["id"])
				assert.InDelta(t, 30, inv0["order"], 0)
				assert.Equal(t, "echo hello; false", inv0["description"])
				assert.InDelta(t, 2, inv0["checks"], 0)
			},
		},
		{
			name: "invalid invariants are reported separately",
			setup: func(t *testing.T) {
				t.Helper()
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
				t.Helper()
				invariants := mustJSONArray(t, data["invariants"])
				require.Len(t, invariants, 1)
				invalid := mustJSONArray(t, data["invalid_invariants"])
				require.Len(t, invalid, 1)
				issue := mustJSONObject(t, invalid[0])
				assert.Equal(t, "broken-order.yaml", issue["file"])
				assert.Equal(t, "order", issue["path"])
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

			output, cmdErr := executeInvariantListCmd(t)
			require.NoError(t, cmdErr)

			var data map[string]any
			require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
			assert.Equal(t, tt.wantStatus, data["status"])

			if tt.checkJSON != nil {
				tt.checkJSON(t, data)
			}
		})
	}
}
