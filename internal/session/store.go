package session

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nextzhou/argus/internal/core"
	"gopkg.in/yaml.v3"
)

// Store persists per-session state for hook and command flows.
type Store interface {
	Load(sessionID string) (*Session, error)
	Save(sessionID string, s *Session) error
	Exists(sessionID string) bool
}

// FileStore persists session state under a filesystem directory.
type FileStore struct {
	baseDir string
}

// NewFileStore returns a Store backed by session YAML files under baseDir.
func NewFileStore(baseDir string) *FileStore {
	return &FileStore{baseDir: baseDir}
}

// LoadSession reads and parses a session data file from disk.
func LoadSession(baseDir, sessionID string) (*Session, error) {
	return NewFileStore(baseDir).Load(sessionID)
}

// Load reads and parses a session data file from disk.
func (store *FileStore) Load(sessionID string) (*Session, error) {
	safeID := core.SessionIDToSafeID(sessionID)
	path := filepath.Join(store.baseDir, safeID+".yaml")

	if err := core.ValidatePath(store.baseDir, path); err != nil {
		return nil, fmt.Errorf("validating session path: %w", err)
	}

	//nolint:gosec // SessionIDToSafeID plus ValidatePath constrain the file to store.baseDir.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening session file %q: %w", sessionID, err)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var loaded Session
	if err := dec.Decode(&loaded); err != nil {
		return nil, fmt.Errorf("parsing session YAML %q: %w", sessionID, err)
	}

	return &loaded, nil
}

// SaveSession writes a session data file to disk, creating the directory if needed.
func SaveSession(baseDir, sessionID string, s *Session) error {
	return NewFileStore(baseDir).Save(sessionID, s)
}

// Save writes a session data file to disk, creating the directory if needed.
func (store *FileStore) Save(sessionID string, sessionData *Session) error {
	if err := os.MkdirAll(store.baseDir, 0o700); err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}

	safeID := core.SessionIDToSafeID(sessionID)
	path := filepath.Join(store.baseDir, safeID+".yaml")

	if err := core.ValidatePath(store.baseDir, path); err != nil {
		return fmt.Errorf("validating session path: %w", err)
	}

	data, err := yaml.Marshal(sessionData)
	if err != nil {
		return fmt.Errorf("marshaling session %q: %w", sessionID, err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing session file %q: %w", sessionID, err)
	}

	return nil
}

// Exists checks whether a session data file exists on disk.
func Exists(baseDir, sessionID string) bool {
	return NewFileStore(baseDir).Exists(sessionID)
}

// Exists reports whether a session data file exists on disk.
func (store *FileStore) Exists(sessionID string) bool {
	safeID := core.SessionIDToSafeID(sessionID)
	path := filepath.Join(store.baseDir, safeID+".yaml")

	if err := core.ValidatePath(store.baseDir, path); err != nil {
		return false
	}

	_, err := os.Stat(path)
	return err == nil
}
