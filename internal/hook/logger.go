package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nextzhou/argus/internal/core"
)

// LogHookExecution writes a single log entry for a hook execution.
// projectRoot is the project root path (empty string triggers global fallback).
// command is the hook command name (e.g. "tick", "trap").
// success indicates whether the hook execution was successful.
// details provides additional context for the log entry.
//
// Log format: {COMPACT_UTC} [{COMMAND}] {OK|ERROR} {DETAILS}
// Example: 20260408T071500Z [tick] OK pipeline: release
//
// File path logic:
// - Primary path: <projectRoot>/.argus/logs/hook.log (when projectRoot is non-empty)
// - Fallback path: ~/.config/argus/logs/hook.log (when projectRoot is empty string)
//
// Parent directories are auto-created if they don't exist.
// File is opened in append mode.
func LogHookExecution(projectRoot string, command string, success bool, details string) error {
	// Determine log file path
	var logPath string
	if projectRoot != "" {
		logPath = filepath.Join(projectRoot, ".argus", "logs", "hook.log")
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
	if err := os.MkdirAll(logDir, 0o755); err != nil {
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
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file %q: %w", logPath, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing log file: %w", closeErr)
		}
	}()

	// Write entry
	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("writing to log file: %w", err)
	}

	return nil
}
