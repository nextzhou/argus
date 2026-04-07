package session

import (
	"slices"
	"testing"
	"time"

	"github.com/nextzhou/argus/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager(t *testing.T) {
	require.NotNil(t, t.Context())

	t.Run("TestIsFirstTick", TestIsFirstTick)
	t.Run("TestAddSnooze", TestAddSnooze)
	t.Run("TestIsSnoozed", TestIsSnoozed)
	t.Run("TestSnoozeAll", TestSnoozeAll)
	t.Run("TestUpdateLastTick", TestUpdateLastTick)
	t.Run("TestHasStateChanged", TestHasStateChanged)
	t.Run("TestSessionFileTimingContract", TestSessionFileTimingContract)
}

func TestIsFirstTick(t *testing.T) {
	require.NotNil(t, t.Context())

	tests := []struct {
		name      string
		saveFirst bool
		want      bool
	}{
		{
			name: "returns true when session file does not exist",
			want: true,
		},
		{
			name:      "returns false when session file exists",
			saveFirst: true,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, t.Context())

			baseDir := t.TempDir()
			sessionID := "session-123"

			if tt.saveFirst {
				err := SaveSession(baseDir, sessionID, &Session{})
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want, IsFirstTick(baseDir, sessionID))
		})
	}
}

func TestAddSnooze(t *testing.T) {
	require.NotNil(t, t.Context())

	t.Run("adds pipeline only once", func(t *testing.T) {
		require.NotNil(t, t.Context())

		s := &Session{}

		AddSnooze(s, "pipeline-a")
		AddSnooze(s, "pipeline-a")

		assert.Equal(t, []string{"pipeline-a"}, s.SnoozedPipelines)
	})
}

func TestIsSnoozed(t *testing.T) {
	require.NotNil(t, t.Context())

	tests := []struct {
		name       string
		snoozed    []string
		pipelineID string
		want       bool
	}{
		{
			name:       "returns true for snoozed pipeline",
			snoozed:    []string{"pipeline-a", "pipeline-b"},
			pipelineID: "pipeline-b",
			want:       true,
		},
		{
			name:       "returns false for pipeline not in snooze list",
			snoozed:    []string{"pipeline-a"},
			pipelineID: "pipeline-b",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, t.Context())

			s := &Session{SnoozedPipelines: slices.Clone(tt.snoozed)}
			assert.Equal(t, tt.want, IsSnoozed(s, tt.pipelineID))
		})
	}
}

func TestSnoozeAll(t *testing.T) {
	require.NotNil(t, t.Context())

	t.Run("adds multiple pipeline ids without duplicates on repeated calls", func(t *testing.T) {
		require.NotNil(t, t.Context())

		s := &Session{SnoozedPipelines: []string{"existing"}}
		pipelineIDs := []string{"pipeline-a", "pipeline-b", "existing"}

		SnoozeAll(s, pipelineIDs)
		SnoozeAll(s, pipelineIDs)

		assert.Len(t, s.SnoozedPipelines, 3)
		assert.True(t, slices.Contains(s.SnoozedPipelines, "existing"))
		assert.True(t, slices.Contains(s.SnoozedPipelines, "pipeline-a"))
		assert.True(t, slices.Contains(s.SnoozedPipelines, "pipeline-b"))
		assert.Equal(t, 1, countMatches(s.SnoozedPipelines, "existing"))
		assert.Equal(t, 1, countMatches(s.SnoozedPipelines, "pipeline-a"))
		assert.Equal(t, 1, countMatches(s.SnoozedPipelines, "pipeline-b"))
	})
}

func TestUpdateLastTick(t *testing.T) {
	require.NotNil(t, t.Context())

	t.Run("populates last tick with formatted timestamp", func(t *testing.T) {
		require.NotNil(t, t.Context())

		s := &Session{}
		now := time.Date(2026, time.April, 7, 12, 34, 56, 789000000, time.FixedZone("UTC+8", 8*60*60))

		UpdateLastTick(s, "pipeline-a", "job-b", now)

		require.NotNil(t, s.LastTick)
		assert.Equal(t, "pipeline-a", s.LastTick.Pipeline)
		assert.Equal(t, "job-b", s.LastTick.Job)
		assert.Equal(t, core.FormatTimestamp(now), s.LastTick.Timestamp)
	})
}

func TestHasStateChanged(t *testing.T) {
	require.NotNil(t, t.Context())

	tests := []struct {
		name            string
		session         *Session
		currentPipeline string
		currentJob      string
		want            bool
	}{
		{
			name:            "returns true when last tick is nil",
			session:         &Session{},
			currentPipeline: "pipeline-a",
			currentJob:      "job-a",
			want:            true,
		},
		{
			name: "returns false when pipeline and job match last tick",
			session: &Session{LastTick: &LastTickState{
				Pipeline: "pipeline-a",
				Job:      "job-a",
			}},
			currentPipeline: "pipeline-a",
			currentJob:      "job-a",
			want:            false,
		},
		{
			name: "returns true when job changed within same pipeline",
			session: &Session{LastTick: &LastTickState{
				Pipeline: "pipeline-a",
				Job:      "job-a",
			}},
			currentPipeline: "pipeline-a",
			currentJob:      "job-b",
			want:            true,
		},
		{
			name: "returns true when pipeline changed",
			session: &Session{LastTick: &LastTickState{
				Pipeline: "pipeline-a",
				Job:      "job-a",
			}},
			currentPipeline: "pipeline-b",
			currentJob:      "job-a",
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, t.Context())
			assert.Equal(t, tt.want, HasStateChanged(tt.session, tt.currentPipeline, tt.currentJob))
		})
	}
}

func TestSessionFileTimingContract(t *testing.T) {
	require.NotNil(t, t.Context())

	baseDir := t.TempDir()
	sessionID := "timing-contract-session"

	assert.True(t, IsFirstTick(baseDir, sessionID))

	err := SaveSession(baseDir, sessionID, &Session{})
	require.NoError(t, err)

	assert.False(t, IsFirstTick(baseDir, sessionID))
}

func countMatches(items []string, target string) int {
	count := 0
	for _, item := range items {
		if item == target {
			count++
		}
	}

	return count
}
