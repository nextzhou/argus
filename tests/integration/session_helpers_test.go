package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/sessiontest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultSessionIDCleansStaleFileAndRegistersCleanup(t *testing.T) {
	var sessionPath string

	t.Run("managed cleanup", func(t *testing.T) {
		expectedSessionID := sessiontest.NewSessionID(t, "stale-helper")
		sessionPath = defaultSessionFilePath(expectedSessionID)

		require.NoError(t, os.MkdirAll(filepath.Dir(sessionPath), 0o755))
		require.NoError(t, os.WriteFile(sessionPath, []byte("stale"), 0o644))
		require.True(t, fileExists(t, sessionPath))

		managedSessionID := newDefaultSessionID(t, "stale-helper")
		assert.Equal(t, expectedSessionID, managedSessionID)
		assert.False(t, fileExists(t, sessionPath))

		require.NoError(t, os.WriteFile(sessionPath, []byte("created during test"), 0o644))
		assert.True(t, fileExists(t, sessionPath))
	})

	assert.False(t, fileExists(t, sessionPath))
}

func TestCleanupDefaultSessionFileUsesSafeID(t *testing.T) {
	sessionID := "non-uuid session/../value"
	sessionPath := defaultSessionFilePath(sessionID)

	require.NoError(t, os.MkdirAll(filepath.Dir(sessionPath), 0o755))
	require.NoError(t, os.WriteFile(sessionPath, []byte("stale"), 0o644))

	t.Run("cleanup by safe id", func(t *testing.T) {
		cleanupDefaultSessionFile(t, sessionID)
		assert.False(t, fileExists(t, sessionPath))

		require.NoError(t, os.WriteFile(sessionPath, []byte("created during test"), 0o644))
		assert.True(t, fileExists(t, sessionPath))
	})

	assert.False(t, fileExists(t, sessionPath))
}

func defaultSessionFilePath(sessionID string) string {
	return filepath.Join("/tmp", "argus", core.SessionIDToSafeID(sessionID)+".yaml")
}
