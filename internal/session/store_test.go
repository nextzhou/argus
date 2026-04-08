package session

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionLifecycle(t *testing.T) {
	dir := t.TempDir()
	sessionID := "abc123-4567-89ab-cdef"

	// Step 1: Exists returns false for nonexistent session
	assert.False(t, Exists(dir, sessionID))

	// Step 2: SaveSession creates the file
	original := &Session{
		SnoozedPipelines: []string{"release-20240115T103000Z"},
		LastTick: &LastTickState{
			Pipeline:  "release-20240115T103000Z",
			Job:       "run_tests",
			Timestamp: "20240115T113000Z",
		},
	}

	err := SaveSession(dir, sessionID, original)
	require.NoError(t, err)

	// Step 3: Exists returns true after save
	assert.True(t, Exists(dir, sessionID))

	// Step 4: LoadSession returns correct data
	loaded, err := LoadSession(dir, sessionID)
	require.NoError(t, err)

	assert.Equal(t, original.SnoozedPipelines, loaded.SnoozedPipelines)
	require.NotNil(t, loaded.LastTick)
	assert.Equal(t, original.LastTick.Pipeline, loaded.LastTick.Pipeline)
	assert.Equal(t, original.LastTick.Job, loaded.LastTick.Job)
	assert.Equal(t, original.LastTick.Timestamp, loaded.LastTick.Timestamp)
}

func TestSessionLifecycleNilLastTick(t *testing.T) {
	dir := t.TempDir()
	sessionID := "aaa-bbb-ccc"

	original := &Session{
		SnoozedPipelines: []string{},
		LastTick:         nil,
	}

	err := SaveSession(dir, sessionID, original)
	require.NoError(t, err)

	loaded, err := LoadSession(dir, sessionID)
	require.NoError(t, err)

	assert.Nil(t, loaded.LastTick)
}

func TestSaveSessionCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")

	s := &Session{}

	err := SaveSession(dir, "abc-def", s)
	require.NoError(t, err)

	// Verify directory was created
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLoadSessionNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadSession(dir, "nonexistent-session")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestLoadSessionInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	sessionID := "abc-123"

	// Write invalid YAML directly
	err := os.WriteFile(filepath.Join(dir, sessionID+".yaml"), []byte("{{invalid yaml"), 0o644)
	require.NoError(t, err)

	_, err = LoadSession(dir, sessionID)
	assert.Error(t, err)
}

func TestLoadSessionUnknownFields(t *testing.T) {
	dir := t.TempDir()
	sessionID := "abc-456"

	content := `snoozed_pipelines: []
unknown_field: should_fail
`
	err := os.WriteFile(filepath.Join(dir, sessionID+".yaml"), []byte(content), 0o644)
	require.NoError(t, err)

	_, err = LoadSession(dir, sessionID)
	assert.Error(t, err)
}

func TestSafeIDPathUUID(t *testing.T) {
	dir := t.TempDir()
	// UUID-like session ID (hex digits and hyphens only) -> used directly
	sessionID := "abc123-4567-89ab-cdef"

	s := &Session{
		SnoozedPipelines: []string{"test-pipeline"},
	}

	err := SaveSession(dir, sessionID, s)
	require.NoError(t, err)

	// UUID-like IDs are used directly as filename
	expectedPath := filepath.Join(dir, sessionID+".yaml")
	_, err = os.Stat(expectedPath)
	assert.NoError(t, err, "UUID session ID should map to direct filename")
}

func TestSafeIDPathNonUUID(t *testing.T) {
	dir := t.TempDir()
	// Non-UUID session ID (contains non-hex chars) -> SHA256 hash prefix
	sessionID := "opencode-session-xyz"

	s := &Session{
		SnoozedPipelines: []string{"test-pipeline"},
	}

	err := SaveSession(dir, sessionID, s)
	require.NoError(t, err)

	// Compute expected safe ID (SHA256 first 16 hex chars)
	hash := sha256.Sum256([]byte(sessionID))
	expectedSafeID := fmt.Sprintf("%x", hash[:8])
	expectedPath := filepath.Join(dir, expectedSafeID+".yaml")

	_, err = os.Stat(expectedPath)
	assert.NoError(t, err, "non-UUID session ID should map to SHA256 hash prefix filename")

	// Verify the direct name file does NOT exist
	directPath := filepath.Join(dir, sessionID+".yaml")
	_, err = os.Stat(directPath)
	assert.True(t, errors.Is(err, os.ErrNotExist), "direct filename should not exist for non-UUID session ID")

	// Verify we can still load via the original session ID
	loaded, err := LoadSession(dir, sessionID)
	require.NoError(t, err)
	assert.Equal(t, []string{"test-pipeline"}, loaded.SnoozedPipelines)
}

func TestExistsReturnsFalseOnError(t *testing.T) {
	// Using an invalid path that will cause os.Stat to fail
	assert.False(t, Exists("", "any-session"))
}

func TestValidatePathDefenseInDepth(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-session"

	s := &Session{
		SnoozedPipelines: []string{"test"},
	}

	err := SaveSession(baseDir, sessionID, s)
	require.NoError(t, err)

	loaded, err := LoadSession(baseDir, sessionID)
	require.NoError(t, err)
	assert.Equal(t, s.SnoozedPipelines, loaded.SnoozedPipelines)

	assert.True(t, Exists(baseDir, sessionID))
}
