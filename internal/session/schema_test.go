package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSessionZeroValue(t *testing.T) {
	var s Session

	assert.Nil(t, s.SnoozedPipelines)
	assert.Nil(t, s.LastTick)
}

func TestLastTickStateZeroValue(t *testing.T) {
	var lt LastTickState

	assert.Empty(t, lt.Pipeline)
	assert.Empty(t, lt.Job)
	assert.Empty(t, lt.Timestamp)
}

func TestSessionFields(t *testing.T) {
	s := Session{
		SnoozedPipelines: []string{"release-20240115T103000Z", "build-20240115T100000Z"},
		LastTick: &LastTickState{
			Pipeline:  "release-20240115T103000Z",
			Job:       "run_tests",
			Timestamp: "20240115T113000Z",
		},
	}

	assert.Len(t, s.SnoozedPipelines, 2)
	assert.Equal(t, "release-20240115T103000Z", s.SnoozedPipelines[0])
	assert.Equal(t, "build-20240115T100000Z", s.SnoozedPipelines[1])

	assert.NotNil(t, s.LastTick)
	assert.Equal(t, "release-20240115T103000Z", s.LastTick.Pipeline)
	assert.Equal(t, "run_tests", s.LastTick.Job)
	assert.Equal(t, "20240115T113000Z", s.LastTick.Timestamp)
}

func TestSessionNullableLastTick(t *testing.T) {
	tests := []struct {
		name     string
		lastTick *LastTickState
	}{
		{
			name:     "nil last tick for fresh session",
			lastTick: nil,
		},
		{
			name: "populated last tick",
			lastTick: &LastTickState{
				Pipeline:  "release-20240115T103000Z",
				Job:       "lint",
				Timestamp: "20240115T103000Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{
				LastTick: tt.lastTick,
			}
			assert.Equal(t, tt.lastTick, s.LastTick)
		})
	}
}
