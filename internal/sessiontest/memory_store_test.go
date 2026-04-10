package sessiontest

import (
	"io/fs"
	"testing"

	"github.com/nextzhou/argus/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStoreLifecycle(t *testing.T) {
	store := NewMemoryStore()
	sessionID := "memory-store-session"

	assert.False(t, store.Exists(sessionID))

	_, err := store.Load(sessionID)
	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrNotExist)

	original := &session.Session{
		SnoozedPipelines: []string{"pipeline-1"},
		LastTick: &session.LastTickState{
			Pipeline:  "pipeline-1",
			Job:       "job-1",
			Timestamp: "20260410T120000Z",
		},
	}
	require.NoError(t, store.Save(sessionID, original))
	assert.True(t, store.Exists(sessionID))

	loaded, err := store.Load(sessionID)
	require.NoError(t, err)
	require.Equal(t, original, loaded)

	loaded.SnoozedPipelines[0] = "mutated"
	loaded.LastTick.Job = "mutated-job"

	reloaded, err := store.Load(sessionID)
	require.NoError(t, err)
	assert.Equal(t, "pipeline-1", reloaded.SnoozedPipelines[0])
	assert.Equal(t, "job-1", reloaded.LastTick.Job)
}

func TestMemoryStoreNormalizesSessionID(t *testing.T) {
	store := NewMemoryStore()
	sessionID := "opencode-session-xyz"

	require.NoError(t, store.Save(sessionID, &session.Session{
		SnoozedPipelines: []string{"pipeline-1"},
	}))

	assert.True(t, store.Exists(sessionID))

	loaded, err := store.Load(sessionID)
	require.NoError(t, err)
	assert.Equal(t, []string{"pipeline-1"}, loaded.SnoozedPipelines)
}
