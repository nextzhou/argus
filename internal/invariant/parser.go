package invariant

import (
	"fmt"
	"io"
	"os"

	"github.com/nextzhou/argus/internal/core"
	"gopkg.in/yaml.v3"
)

var validAutoValues = map[string]bool{
	"":              true,
	"always":        true,
	"session_start": true,
	"never":         true,
}

// ParseInvariant decodes and validates an invariant definition from the given reader.
// Unknown YAML fields are rejected.
func ParseInvariant(r io.Reader) (*Invariant, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)

	var inv Invariant
	if err := dec.Decode(&inv); err != nil {
		return nil, fmt.Errorf("parsing invariant YAML: %w", err)
	}

	if err := validateInvariant(&inv); err != nil {
		return nil, err
	}

	return &inv, nil
}

// ParseInvariantFile parses an invariant definition from the file at the given path.
func ParseInvariantFile(path string) (*Invariant, error) {
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

func validateInvariant(inv *Invariant) error {
	if err := core.CheckCompatibility(inv.Version); err != nil {
		return fmt.Errorf("invariant version check: %w", err)
	}

	if inv.ID == "" {
		return fmt.Errorf("invariant ID cannot be empty: %w", core.ErrInvalidID)
	}
	if err := core.ValidateWorkflowID(inv.ID); err != nil {
		return fmt.Errorf("invariant %q: %w", inv.ID, err)
	}

	if !validAutoValues[inv.Auto] {
		return fmt.Errorf("invariant %q: auto value %q must be one of: always, session_start, never", inv.ID, inv.Auto)
	}

	if len(inv.Check) == 0 {
		return fmt.Errorf("invariant %q must have at least one check step", inv.ID)
	}

	if inv.Prompt == "" && inv.Workflow == "" {
		return fmt.Errorf("invariant %q must have a prompt or workflow (or both)", inv.ID)
	}

	return nil
}
