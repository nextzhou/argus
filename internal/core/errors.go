// Package core provides shared domain types and utilities for Argus.
package core

import (
	"errors"
	"fmt"
)

// Sentinel errors for known error conditions.
var (
	ErrNotFound             = errors.New("not found")
	ErrInvalidID            = errors.New("invalid ID")
	ErrVersionMismatch      = errors.New("version mismatch")
	ErrNoActivePipeline     = errors.New("no active pipeline")
	ErrActivePipelineExists = errors.New("active pipeline already exists")
)

// ValidationError represents a validation failure for a specific field.
// Use errors.As to extract the Field and Message:
//
//	var ve *ValidationError
//	if errors.As(err, &ve) {
//	    fmt.Println(ve.Field, ve.Message)
//	}
//
// Recommended wrapping pattern: fmt.Errorf("loading workflow: %w", err)
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation: %s: %s", e.Field, e.Message)
}
