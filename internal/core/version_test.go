package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckCompatibility(t *testing.T) {
	tests := []struct {
		name         string
		fileVersion  string
		wantErr      bool
		wantMismatch bool // true if expected ErrVersionMismatch
	}{
		// Compatible: same major (0)
		{"current-version", "v0.1.0", false, false},
		{"minor-higher", "v0.2.0", false, false},
		{"minor-lower", "v0.0.1", false, false},
		{"large-minor", "v0.99.99", false, false},
		// Incompatible: different major
		{"major-1", "v1.0.0", true, true},
		{"major-2", "v2.0.0", true, true},
		{"major-1-minor-5", "v1.5.3", true, true},
		// Malformed: not ErrVersionMismatch
		{"empty", "", true, false},
		{"no-v-prefix", "0.1.0", true, false},
		{"only-major", "v1", true, false},
		{"only-major-minor", "v1.0", true, false},
		{"pre-release", "v0.1.0-beta", true, false},
		{"build-metadata", "v0.1.0+build.1", true, false},
		{"random-text", "invalid", true, false},
		{"non-integer-major", "vx.1.0", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckCompatibility(tt.fileVersion)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			if tt.wantMismatch {
				assert.ErrorIs(t, err, ErrVersionMismatch,
					"expected ErrVersionMismatch, got: %v", err)
			} else {
				assert.NotErrorIs(t, err, ErrVersionMismatch,
					"expected format error (not ErrVersionMismatch), got: %v", err)
			}
		})
	}
}

func TestSchemaVersionIsValid(t *testing.T) {
	assert.Equal(t, "v0.1.0", SchemaVersion)
	// SchemaVersion itself must be compatible
	err := CheckCompatibility(SchemaVersion)
	require.NoError(t, err)
}
