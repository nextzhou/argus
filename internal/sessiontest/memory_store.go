// Package sessiontest provides session store helpers for tests.
package sessiontest

import (
	"fmt"
	"io/fs"
	"slices"
	"sync"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/session"
)

// MemoryStore keeps session state in memory for same-process tests.
type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*session.Session
}

// NewMemoryStore returns an empty in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]*session.Session),
	}
}

// Load returns a copy of the stored session state.
func (m *MemoryStore) Load(sessionID string) (*session.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stored, ok := m.sessions[core.SessionIDToSafeID(sessionID)]
	if !ok {
		return nil, fmt.Errorf("loading session %q: %w", sessionID, fs.ErrNotExist)
	}

	return cloneSession(stored), nil
}

// Save stores a copy of session state under the normalized session ID.
func (m *MemoryStore) Save(sessionID string, s *session.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[core.SessionIDToSafeID(sessionID)] = cloneSession(s)
	return nil
}

// Exists reports whether normalized session state is present.
func (m *MemoryStore) Exists(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.sessions[core.SessionIDToSafeID(sessionID)]
	return ok
}

func cloneSession(s *session.Session) *session.Session {
	if s == nil {
		return &session.Session{}
	}

	cloned := &session.Session{
		SnoozedPipelines: slices.Clone(s.SnoozedPipelines),
	}
	if s.LastTick != nil {
		cloned.LastTick = &session.LastTickState{
			Pipeline:  s.LastTick.Pipeline,
			Job:       s.LastTick.Job,
			Timestamp: s.LastTick.Timestamp,
		}
	}

	return cloned
}
