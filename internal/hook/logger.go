package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nextzhou/argus/internal/core"
)

// LogHookExecution writes a single log entry for a hook execution.
// command is the hook command name (e.g. "tick", "trap").
// success indicates whether the hook execution was successful.
// details provides additional context for the log entry.
//
// Log format: {COMPACT_UTC} [{COMMAND}] {OK|ERROR} {DETAILS}
// Example: 20260408T071500Z [tick] OK pipeline: release
//
// Parent directories are auto-created if they don't exist.
// File is opened in append mode.
func LogHookExecution(logsDir string, command string, success bool, details string) (err error) {
	// Determine log file path
	var logPath string
	if logsDir != "" {
		logPath = filepath.Join(logsDir, "hook.log")
	} else {
		// Fallback to global user-level directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		logPath = filepath.Join(homeDir, ".config", "argus", "logs", "hook.log")
	}

	// Create parent directories if they don't exist
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("creating log directory %q: %w", logDir, err)
	}

	// Format status
	status := "OK"
	if !success {
		status = "ERROR"
	}

	// Format log entry
	timestamp := core.FormatTimestamp(time.Now())
	entry := fmt.Sprintf("%s [%s] %s %s\n", timestamp, command, status, details)

	// Open file in append mode
	//nolint:gosec // logPath is constructed from the hook log directory selected by the caller.
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening log file %q: %w", logPath, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing log file: %w", closeErr)
		}
	}()

	// Write entry
	if _, writeErr := f.WriteString(entry); writeErr != nil {
		return fmt.Errorf("writing to log file: %w", writeErr)
	}

	return nil
}
