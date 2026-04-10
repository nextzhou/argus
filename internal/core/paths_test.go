package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionIDToSafeID(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		wantDirect bool
	}{
		{
			name:       "UUID passthrough",
			sessionID:  "550e8400-e29b-41d4-a716-446655440000",
			wantDirect: true,
		},
		{
			name:       "hex-only string",
			sessionID:  "abcdef1234567890",
			wantDirect: true,
		},
		{
			name:       "hyphens-only matches pattern",
			sessionID:  "----",
			wantDirect: true,
		},
		{
			name:       "empty string goes to hash",
			sessionID:  "",
			wantDirect: false,
		},
		{
			name:       "contains slash - hash",
			sessionID:  "abc/../../etc",
			wantDirect: false,
		},
		{
			name:       "contains space - hash",
			sessionID:  "session id",
			wantDirect: false,
		},
		{
			name:       "non-hex chars - hash",
			sessionID:  "not-a-uuid-!!!",
			wantDirect: false,
		},
		{
			name:       "mixed case hex - direct",
			sessionID:  "ABCDEF-abcdef",
			wantDirect: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SessionIDToSafeID(tt.sessionID)
			if tt.wantDirect {
				assert.Equal(t, tt.sessionID, result)
			} else {
				// Hash result must be 16 hex chars
				assert.Len(t, result, 16)
				for _, c := range result {
					assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
						"hash output must be lowercase hex, got char %q", c)
				}
			}
		})
	}

	t.Run("hash is deterministic", func(t *testing.T) {
		id := "some-non-uuid-id"
		first := SessionIDToSafeID(id)
		second := SessionIDToSafeID(id)
		assert.Equal(t, first, second)
	})
}

func TestProjectPathToSafeID(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "absolute project path",
			path: "/Users/example/work/argus",
		},
		{
			name: "path with spaces",
			path: "/Users/example/My Projects/argus",
		},
		{
			name: "relative-ish traversal input",
			path: "../../tmp/session-store",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProjectPathToSafeID(tt.path)
			assert.Len(t, result, 16)
			assert.Regexp(t, "^[0-9a-f]{16}$", result)
			assert.Equal(t, result, ProjectPathToSafeID(tt.path))
		})
	}

	t.Run("different paths produce different ids", func(t *testing.T) {
		first := ProjectPathToSafeID("/projects/argus-a")
		second := ProjectPathToSafeID("/projects/argus-b")
		assert.NotEqual(t, first, second)
	})
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		target  string
		wantErr bool
	}{
		// Legitimate paths
		{
			name:    "direct child file",
			base:    "/base",
			target:  "/base/file.yaml",
			wantErr: false,
		},
		{
			name:    "nested subdirectory",
			base:    "/base",
			target:  "/base/sub/file.yaml",
			wantErr: false,
		},
		{
			name:    "same as base",
			base:    "/base",
			target:  "/base",
			wantErr: false,
		},
		{
			name:    "target with dot but resolves within",
			base:    "/base",
			target:  "/base/sub/../file.yaml",
			wantErr: false,
		},
		// Traversal attacks
		{
			name:    "simple traversal",
			base:    "/base",
			target:  "/base/../etc/passwd",
			wantErr: true,
		},
		{
			name:    "completely outside base",
			base:    "/base",
			target:  "/other/file",
			wantErr: true,
		},
		{
			name:    "parent of base",
			base:    "/base",
			target:  "/",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.base, tt.target)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "escapes base directory")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
