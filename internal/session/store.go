package session

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nextzhou/argus/internal/core"
	"gopkg.in/yaml.v3"
)

// LoadSession reads and parses a session data file from disk.
func LoadSession(baseDir, sessionID string) (*Session, error) {
	safeID := core.SessionIDToSafeID(sessionID)
	path := filepath.Join(baseDir, safeID+".yaml")

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening session file %q: %w", sessionID, err)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var s Session
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("parsing session YAML %q: %w", sessionID, err)
	}

	return &s, nil
}

// SaveSession writes a session data file to disk, creating the directory if needed.
func SaveSession(baseDir, sessionID string, s *Session) error {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}

	safeID := core.SessionIDToSafeID(sessionID)
	path := filepath.Join(baseDir, safeID+".yaml")

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshaling session %q: %w", sessionID, err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing session file %q: %w", sessionID, err)
	}

	return nil
}

// Exists checks whether a session data file exists on disk.
func Exists(baseDir, sessionID string) bool {
	safeID := core.SessionIDToSafeID(sessionID)
	path := filepath.Join(baseDir, safeID+".yaml")

	_, err := os.Stat(path)
	return err == nil
}
