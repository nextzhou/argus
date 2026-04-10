package sessiontest

import (
	"fmt"
	"testing"

	"github.com/nextzhou/argus/internal/core"
)

// NewSessionID returns a readable, per-test session ID for test inputs.
// The returned value is stable within a test and unique across differently named tests.
func NewSessionID(t testing.TB, label string) string {
	t.Helper()

	if label == "" {
		label = "session"
	}

	return fmt.Sprintf("%s-%s", label, core.SessionIDToSafeID(t.Name()))
}
