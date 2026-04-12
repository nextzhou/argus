package invariant

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nextzhou/argus/internal/core"
	"gopkg.in/yaml.v3"
)

// FieldError represents a single schema validation error in an invariant.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError reports one or more schema validation failures in an invariant.
type ValidationError struct {
	Errors []FieldError
	causes []error
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return ""
	}

	parts := make([]string, 0, len(e.Errors))
	for _, fieldErr := range e.Errors {
		if fieldErr.Path == "" {
			parts = append(parts, fieldErr.Message)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", fieldErr.Path, fieldErr.Message))
	}

	return strings.Join(parts, "; ")
}

func (e *ValidationError) Unwrap() []error {
	if e == nil || len(e.causes) == 0 {
		return nil
	}
	return append([]error(nil), e.causes...)
}

// ParseInvariant decodes and validates an invariant definition from the given reader.
// Unknown YAML fields are rejected.
func ParseInvariant(r io.Reader) (*Invariant, error) {
	inv, err := decodeInvariant(r)
	if err != nil {
		return nil, err
	}

	if err := validateInvariant(inv); err != nil {
		return nil, err
	}

	return inv, nil
}

// ParseInvariantFile parses an invariant definition from the file at the given path.
func ParseInvariantFile(path string) (*Invariant, error) {
	//nolint:gosec // ParseInvariantFile intentionally reads the exact file path selected by the caller.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening invariant file: %w", err)
	}
	defer func() { _ = f.Close() }()

	inv, err := ParseInvariant(f)
	if err != nil {
		return nil, fmt.Errorf("parsing invariant file %q: %w", path, err)
	}
	return inv, nil
}

func decodeInvariant(r io.Reader) (*Invariant, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)

	var inv Invariant
	if err := dec.Decode(&inv); err != nil {
		return nil, fmt.Errorf("parsing invariant YAML: %w", err)
	}

	return &inv, nil
}

func validateInvariant(inv *Invariant) error {
	issues := validationIssues(inv)
	if len(issues) > 0 {
		fieldErrors := make([]FieldError, 0, len(issues))
		causes := make([]error, 0, len(issues))
		for _, issue := range issues {
			fieldErrors = append(fieldErrors, issue.fieldErr)
			if issue.cause != nil {
				causes = append(causes, issue.cause)
			}
		}
		return &ValidationError{Errors: fieldErrors, causes: causes}
	}

	return nil
}

func validationErrors(inv *Invariant) []FieldError {
	issues := validationIssues(inv)
	errs := make([]FieldError, 0, len(issues))
	for _, issue := range issues {
		errs = append(errs, issue.fieldErr)
	}
	return errs
}

type validationIssue struct {
	fieldErr FieldError
	cause    error
}

func validationIssues(inv *Invariant) []validationIssue {
	if inv == nil {
		return []validationIssue{{fieldErr: FieldError{Message: "invariant is nil"}}}
	}

	var errs []validationIssue

	if err := core.CheckCompatibility(inv.Version); err != nil {
		errs = append(errs, validationIssue{
			fieldErr: FieldError{
				Path:    "version",
				Message: fmt.Sprintf("invariant version check: %v", err),
			},
			cause: err,
		})
	}

	if inv.ID == "" {
		errs = append(errs, validationIssue{
			fieldErr: FieldError{
				Path:    "id",
				Message: fmt.Sprintf("invariant ID cannot be empty: %v", core.ErrInvalidID),
			},
			cause: core.ErrInvalidID,
		})
	} else if err := core.ValidateWorkflowID(inv.ID); err != nil {
		errs = append(errs, validationIssue{
			fieldErr: FieldError{
				Path:    "id",
				Message: fmt.Sprintf("invariant %q: %v", inv.ID, err),
			},
			cause: err,
		})
	}

	if inv.Order < 1 {
		errs = append(errs, validationIssue{
			fieldErr: FieldError{
				Path:    "order",
				Message: "order must be a positive integer",
			},
		})
	}

	if !isValidAutoValue(inv.Auto) {
		errs = append(errs, validationIssue{
			fieldErr: FieldError{
				Path:    "auto",
				Message: fmt.Sprintf("invariant %q: auto value %q must be one of: always, session_start, never", inv.ID, inv.Auto),
			},
		})
	}

	if len(inv.Check) == 0 {
		errs = append(errs, validationIssue{
			fieldErr: FieldError{
				Path:    "check",
				Message: fmt.Sprintf("invariant %q must have at least one check step", inv.ID),
			},
		})
	}

	if inv.Prompt == "" && inv.Workflow == "" {
		errs = append(errs, validationIssue{
			fieldErr: FieldError{
				Path:    "prompt",
				Message: fmt.Sprintf("invariant %q must have a prompt or workflow (or both)", inv.ID),
			},
		})
	}

	return errs
}

func isValidAutoValue(value string) bool {
	switch value {
	case "", "always", "session_start", "never":
		return true
	default:
		return false
	}
}
