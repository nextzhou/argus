package core

import (
	"fmt"
	"time"
)

// TimestampFormat is the compact UTC format used throughout Argus.
// Format: YYYYMMDDTHHMMSSZ (example: 20240115T103000Z)
// Used in pipeline started_at/ended_at, pipeline instance IDs, session last_tick, hook logs.
const TimestampFormat = "20060102T150405Z"

// FormatTimestamp converts a time.Time to the compact UTC timestamp format.
// Any timezone is converted to UTC. Nanoseconds are truncated (not rounded).
func FormatTimestamp(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(TimestampFormat)
}

// ParseTimestamp parses a compact UTC timestamp string into a time.Time.
// Returns an error for any malformed input.
func ParseTimestamp(s string) (time.Time, error) {
	t, err := time.Parse(TimestampFormat, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing timestamp %q: %w", s, err)
	}
	return t.UTC(), nil
}
