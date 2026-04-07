package session

import (
	"slices"
	"time"

	"github.com/nextzhou/argus/internal/core"
)

// Session manager provides building blocks for session lifecycle operations.
//
// Timing contract for first-tick invariant checks:
// The caller (tick handler) MUST follow this sequence:
//  1. Call IsFirstTick to check if this is the first tick for the session
//  2. If true, run invariant checks (auto != "never")
//  3. Call SaveSession to persist the session, creating the session file
//
// SaveSession MUST only be called AFTER invariant checks complete.
// IsFirstTick relies on session file existence; saving before checks
// would cause subsequent ticks to skip invariant checks incorrectly.

// IsFirstTick reports whether the session file does not exist yet.
// This is part of the timing contract: a true return means invariant
// checks should run before the session file is created via SaveSession.
func IsFirstTick(baseDir, sessionID string) bool {
	return !Exists(baseDir, sessionID)
}

// AddSnooze appends a pipeline ID unless it is already snoozed.
func AddSnooze(s *Session, pipelineID string) {
	if s == nil || slices.Contains(s.SnoozedPipelines, pipelineID) {
		return
	}

	s.SnoozedPipelines = append(s.SnoozedPipelines, pipelineID)
}

// IsSnoozed reports whether a pipeline ID is in the snooze list.
func IsSnoozed(s *Session, pipelineID string) bool {
	if s == nil {
		return false
	}

	return slices.Contains(s.SnoozedPipelines, pipelineID)
}

// SnoozeAll adds each pipeline ID to the snooze list once.
func SnoozeAll(s *Session, pipelineIDs []string) {
	for _, pipelineID := range pipelineIDs {
		AddSnooze(s, pipelineID)
	}
}

// UpdateLastTick stores the latest pipeline/job snapshot for the session.
func UpdateLastTick(s *Session, pipelineID, jobID string, now time.Time) {
	if s == nil {
		return
	}

	s.LastTick = &LastTickState{
		Pipeline:  pipelineID,
		Job:       jobID,
		Timestamp: core.FormatTimestamp(now),
	}
}

// HasStateChanged reports whether the pipeline/job differs from the last tick.
func HasStateChanged(s *Session, currentPipeline, currentJob string) bool {
	if s == nil || s.LastTick == nil {
		return true
	}

	if s.LastTick.Pipeline != currentPipeline {
		return true
	}

	return s.LastTick.Job != currentJob
}
