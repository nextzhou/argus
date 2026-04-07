package core

import (
	"fmt"
	"strconv"
	"strings"
)

// SchemaVersion is the current schema version for Argus data files.
// All workflow YAML, invariant YAML, and pipeline data files must include this version.
const SchemaVersion = "v0.1.0"

// CheckCompatibility checks if a file's version is compatible with the current schema version.
// Compatibility is determined by major version matching:
//   - v0.x.y files are compatible with v0.y.z (both major=0)
//   - v1.x.y files are NOT compatible with v0.y.z (different major)
//
// Note: this deliberately differs from semver semantics. For Argus, all v0.x.y files
// are mutually compatible, as the schema is still evolving.
//
// Returns nil if compatible, ErrVersionMismatch if major versions differ,
// or a format error if the version string is malformed.
func CheckCompatibility(fileVersion string) error {
	currentMajor, err := parseMajor(SchemaVersion)
	if err != nil {
		// This should never happen with a valid SchemaVersion constant
		return fmt.Errorf("parsing current schema version: %w", err)
	}

	fileMajor, err := parseMajor(fileVersion)
	if err != nil {
		return fmt.Errorf("parsing file version %q: %w", fileVersion, err)
	}

	if fileMajor != currentMajor {
		return fmt.Errorf("file version %q (major=%d) incompatible with schema version %q (major=%d): %w",
			fileVersion, fileMajor, SchemaVersion, currentMajor, ErrVersionMismatch)
	}
	return nil
}

// parseMajor parses the major version number from a version string like "v1.2.3".
// Returns an error for malformed input (missing v prefix, wrong number of parts, non-integer).
func parseMajor(version string) (int, error) {
	if !strings.HasPrefix(version, "v") {
		return 0, fmt.Errorf("version %q must start with 'v'", version)
	}
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("version %q must have format v{{major}}.{{minor}}.{{patch}}", version)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("version %q major part %q is not an integer: %w", version, parts[0], err)
	}
	// Validate minor and patch are also integers
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return 0, fmt.Errorf("version %q minor part %q is not an integer: %w", version, parts[1], err)
	}
	if _, err := strconv.Atoi(parts[2]); err != nil {
		return 0, fmt.Errorf("version %q patch part %q is not an integer: %w", version, parts[2], err)
	}
	return major, nil
}
