package main

import (
	"encoding/json"
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
				invariants, ok := data["invariants"].([]any)
				require.True(t, ok, "invariants should be an array")
				assert.Empty(t, invariants)
			},
		},
		{
			name: "list all invariants sorted by id",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "check-pass", passingInvariant)
				writeInvariantFixture(t, "check-fail", failingInvariant)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				invariants, ok := data["invariants"].([]any)
				require.True(t, ok, "invariants should be an array")
				require.Len(t, invariants, 2)

				inv0 := invariants[0].(map[string]any)
				assert.Equal(t, "check-fail", inv0["id"])
				assert.Equal(t, "Always fails", inv0["description"])
				assert.Equal(t, "never", inv0["auto"])
				assert.Equal(t, float64(1), inv0["checks"])

				inv1 := invariants[1].(map[string]any)
				assert.Equal(t, "check-pass", inv1["id"])
				assert.Equal(t, "Always passes", inv1["description"])
				assert.Equal(t, "always", inv1["auto"])
				assert.Equal(t, float64(1), inv1["checks"])
			},
		},
		{
			name: "description fallback to shell commands",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "no-desc", failingNoDescription)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				invariants := data["invariants"].([]any)
				require.Len(t, invariants, 1)
				inv0 := invariants[0].(map[string]any)
				assert.Equal(t, "no-desc", inv0["id"])
				assert.Equal(t, "echo hello; false", inv0["description"])
				assert.Equal(t, float64(2), inv0["checks"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

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
