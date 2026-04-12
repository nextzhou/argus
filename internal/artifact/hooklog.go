package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nextzhou/argus/internal/core"
)

// HookLogStore appends hook execution logs for one namespace.
type HookLogStore struct {
	projectRoot string
	path        string
}

// NewHookLogStore creates a hook log store for one artifact namespace.
func NewHookLogStore(projectRoot, path string) *HookLogStore {
	return &HookLogStore{
		projectRoot: projectRoot,
		path:        path,
	}
}

// ProjectRoot returns the project root used for relative rendering and policy.
func (s *HookLogStore) ProjectRoot() string {
	if s == nil {
		return ""
	}
	return s.projectRoot
}

// Append appends one hook execution record to the backing log file.
func (s *HookLogStore) Append(command string, success bool, details string) (err error) {
	if s == nil {
		return fmt.Errorf("hook log store is nil")
	}

	logDir := filepath.Dir(s.path)
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("creating log directory %q: %w", logDir, err)
	}

	status := "OK"
	if !success {
		status = "ERROR"
	}

	entry := fmt.Sprintf("%s [%s] %s %s\n", core.FormatTimestamp(time.Now()), command, status, details)

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening log file %q: %w", s.path, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing log file: %w", closeErr)
		}
	}()

	if _, writeErr := f.WriteString(entry); writeErr != nil {
		return fmt.Errorf("writing to log file: %w", writeErr)
	}

	return nil
}
