package core

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wrapped error
	}{
		{"ErrNotFound", ErrNotFound, fmt.Errorf("wrap: %w", ErrNotFound)},
		{"ErrInvalidID", ErrInvalidID, fmt.Errorf("wrap: %w", ErrInvalidID)},
		{"ErrVersionMismatch", ErrVersionMismatch, fmt.Errorf("wrap: %w", ErrVersionMismatch)},
		{"ErrNoActivePipeline", ErrNoActivePipeline, fmt.Errorf("wrap: %w", ErrNoActivePipeline)},
		{"ErrActivePipelineExists", ErrActivePipelineExists, fmt.Errorf("wrap: %w", ErrActivePipelineExists)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrapped match
			require.ErrorIs(t, tt.wrapped, tt.err)
			// Error() is non-empty
			assert.NotEmpty(t, tt.err.Error())
		})
	}
}

func TestValidationError(t *testing.T) {
	ve := &ValidationError{Field: "workflow_id", Message: "must not be empty"}
	assert.Contains(t, ve.Error(), "workflow_id")
	assert.Contains(t, ve.Error(), "must not be empty")
	assert.Contains(t, ve.Error(), "validation")

	// errors.As extraction
	wrapped := fmt.Errorf("context: %w", ve)
	var extracted *ValidationError
	require.ErrorAs(t, wrapped, &extracted)
	assert.Equal(t, "workflow_id", extracted.Field)
	assert.Equal(t, "must not be empty", extracted.Message)

	// ValidationError does NOT match sentinel errors
	require.NotErrorIs(t, ve, ErrNotFound)
	require.NotErrorIs(t, ve, ErrInvalidID)
}

func TestWrappingPreservesSentinel(t *testing.T) {
	original := ErrNotFound
	wrapped := fmt.Errorf("loading config: %w", original)
	doubleWrapped := fmt.Errorf("outer: %w", wrapped)

	require.ErrorIs(t, doubleWrapped, ErrNotFound)
	require.ErrorIs(t, wrapped, ErrNotFound)
}
