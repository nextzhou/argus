package core

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// uuidPattern matches UUID format and UUID-like strings (hex digits and hyphens).
// Session IDs matching this pattern are used directly as safe file names.
// Others are hashed to prevent path traversal.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F-]+$`)

// SessionIDToSafeID converts a session ID to a safe file name component.
//
// Algorithm:
//   - If sessionID matches UUID format (hex digits and hyphens only): return as-is
//   - Otherwise: compute SHA256 hash and return the first 16 hex characters
//
// Path construction rules for reference (used by M2 store modules):
//   - Pipeline files: .argus/pipelines/<workflow-id>-<timestamp>.yaml
//   - Session files:  /tmp/argus/<safe-id>.yaml
//   - Invariant files: .argus/invariants/<id>.yaml
func SessionIDToSafeID(sessionID string) string {
	if uuidPattern.MatchString(sessionID) {
		return sessionID
	}
	hash := sha256.Sum256([]byte(sessionID))
	return fmt.Sprintf("%x", hash[:8])
}

// ProjectPathToSafeID hashes a project path into a deterministic safe directory ID.
func ProjectPathToSafeID(projectPath string) string {
	hash := sha256.Sum256([]byte(projectPath))
	return fmt.Sprintf("%x", hash[:8])
}

// ValidatePath checks that target is within the base directory.
// Returns an error if target escapes base via path traversal (e.g., "../").
//
// Known limitation: symlinks are not resolved (Phase 1 constraint).
func ValidatePath(base, target string) error {
	cleanTarget := filepath.Clean(target)
	rel, err := filepath.Rel(base, cleanTarget)
	if err != nil {
		return fmt.Errorf("computing relative path from %q to %q: %w", base, target, err)
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path %q escapes base directory %q", target, base)
	}
	return nil
}
