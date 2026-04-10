package sessiontest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSessionIDIncludesLabel(t *testing.T) {
	sessionID := NewSessionID(t, "tick")

	assert.Contains(t, sessionID, "tick-")
}

func TestNewSessionIDUsesDefaultLabel(t *testing.T) {
	sessionID := NewSessionID(t, "")

	assert.Contains(t, sessionID, "session-")
}
