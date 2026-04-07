package core

import (
	"errors"
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
			// Direct match
			assert.ErrorIs(t, tt.err, tt.err)
			// Wrapped match
			assert.ErrorIs(t, tt.wrapped, tt.err)
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
	require.True(t, errors.As(wrapped, &extracted))
	assert.Equal(t, "workflow_id", extracted.Field)
	assert.Equal(t, "must not be empty", extracted.Message)

	// ValidationError does NOT match sentinel errors
	assert.False(t, errors.Is(ve, ErrNotFound))
	assert.False(t, errors.Is(ve, ErrInvalidID))
}

func TestWrappingPreservesSentinel(t *testing.T) {
	original := ErrNotFound
	wrapped := fmt.Errorf("loading config: %w", original)
	doubleWrapped := fmt.Errorf("outer: %w", wrapped)

	assert.ErrorIs(t, doubleWrapped, ErrNotFound)
	assert.ErrorIs(t, wrapped, ErrNotFound)
}
