package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		name  string
		input time.Time
		want  string
	}{
		{
			name:  "UTC time",
			input: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			want:  "20240115T103000Z",
		},
		{
			name:  "non-UTC timezone (CST +8)",
			input: time.Date(2024, 1, 15, 18, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			want:  "20240115T103000Z", // converted to UTC
		},
		{
			name:  "nanoseconds truncated",
			input: time.Date(2024, 1, 15, 10, 30, 0, 999999999, time.UTC),
			want:  "20240115T103000Z", // nanoseconds dropped
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTimestamp(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantUTC time.Time
	}{
		{
			name:    "valid timestamp",
			input:   "20240115T103000Z",
			wantErr: false,
			wantUTC: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "random text",
			input:   "not-a-timestamp",
			wantErr: true,
		},
		{
			name:    "partial format",
			input:   "20240115",
			wantErr: true,
		},
		{
			name:    "ISO 8601 format (not our format)",
			input:   "2024-01-15T10:30:00Z",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimestamp(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "parsing timestamp")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUTC, got)
			assert.Equal(t, time.UTC, got.Location())
		})
	}
}

func TestTimestampRoundTrip(t *testing.T) {
	// Round-trip: ParseTimestamp(FormatTimestamp(t)) == t (truncated to second)
	original := time.Date(2024, 6, 15, 14, 30, 45, 123456789, time.UTC)
	formatted := FormatTimestamp(original)
	parsed, err := ParseTimestamp(formatted)
	require.NoError(t, err)

	// Should equal original truncated to second
	expected := original.Truncate(time.Second)
	assert.Equal(t, expected, parsed)
}
