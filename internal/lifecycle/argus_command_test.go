package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsArgusCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{
			name:    "direct argus command",
			command: "argus tick --agent claude-code",
			want:    true,
		},
		{
			name:    "direct absolute argus path",
			command: "/tmp/bin/argus trap --agent claude-code",
			want:    true,
		},
		{
			name:    "exec direct argus command",
			command: "exec argus version",
			want:    true,
		},
		{
			name:    "managed wrapper command",
			command: "if ! command -v argus >/dev/null 2>&1; then printf '%s\\n' 'Argus: Please install Argus CLI. See project README for instructions.'; exit 0; fi; exec argus tick --agent claude-code",
			want:    true,
		},
		{
			name:    "custom wrapper that mentions argus is not direct command",
			command: "bash -lc 'argus tick --agent claude-code'",
			want:    false,
		},
		{
			name:    "non argus command",
			command: "custom-tool tick --agent claude-code",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsArgusCommand(tt.command))
		})
	}
}

func TestIsArgusAgentCommand(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		subcommand string
		agent      string
		want       bool
	}{
		{
			name:       "matches direct tick command",
			command:    "argus tick --agent claude-code",
			subcommand: "tick",
			agent:      "claude-code",
			want:       true,
		},
		{
			name:       "matches managed wrapper tick command",
			command:    "if ! command -v argus >/dev/null 2>&1; then printf '%s\\n' 'Argus: Please install Argus CLI. See project README for instructions.'; exit 0; fi; exec argus tick --agent codex --global",
			subcommand: "tick",
			agent:      "codex",
			want:       true,
		},
		{
			name:       "rejects wrong agent",
			command:    "argus tick --agent codex",
			subcommand: "tick",
			agent:      "claude-code",
			want:       false,
		},
		{
			name:       "rejects wrong subcommand",
			command:    "argus trap --agent claude-code",
			subcommand: "tick",
			agent:      "claude-code",
			want:       false,
		},
		{
			name:       "rejects custom wrapper",
			command:    "bash -lc 'argus tick --agent claude-code'",
			subcommand: "tick",
			agent:      "claude-code",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsArgusAgentCommand(tt.command, tt.subcommand, tt.agent))
		})
	}
}
