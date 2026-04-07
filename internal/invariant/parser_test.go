package invariant

import (
	"errors"
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInvariant(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantID     string
		wantAuto   string
		wantChecks int
	}{
		{
			name: "auto always",
			yaml: `
version: v0.1.0
id: argus-init
description: "project initialized"
auto: always
check:
  - shell: "test -d .argus/rules"
    description: "Rules dir exists"
workflow: argus-init
`,
			wantID:     "argus-init",
			wantAuto:   "always",
			wantChecks: 1,
		},
		{
			name: "auto session_start",
			yaml: `
version: v0.1.0
id: lint-clean
auto: session_start
check:
  - shell: "test -f .lint-passed"
prompt: "Please run lint"
workflow: run-lint
`,
			wantID:     "lint-clean",
			wantAuto:   "session_start",
			wantChecks: 1,
		},
		{
			name: "auto never",
			yaml: `
version: v0.1.0
id: agents-md-fresh
auto: never
check:
  - shell: "find AGENTS.md -mtime -7 | grep -q ."
workflow: update-agents-md
`,
			wantID:     "agents-md-fresh",
			wantAuto:   "never",
			wantChecks: 1,
		},
		{
			name: "no auto field stays empty",
			yaml: `
version: v0.1.0
id: my-check
check:
  - shell: "test -f README.md"
prompt: "Create a README"
`,
			wantID:     "my-check",
			wantAuto:   "",
			wantChecks: 1,
		},
		{
			name: "both prompt and workflow",
			yaml: `
version: v0.1.0
id: lint-clean
check:
  - shell: "test -f .lint-passed"
prompt: "Please run lint"
workflow: run-lint
`,
			wantID:     "lint-clean",
			wantAuto:   "",
			wantChecks: 1,
		},
		{
			name: "only prompt",
			yaml: `
version: v0.1.0
id: gitignore-check
check:
  - shell: "grep -q '.argus/logs' .gitignore"
prompt: "Add .argus/logs/ to .gitignore"
`,
			wantID:     "gitignore-check",
			wantAuto:   "",
			wantChecks: 1,
		},
		{
			name: "only workflow",
			yaml: `
version: v0.1.0
id: agents-md-fresh
check:
  - shell: "find AGENTS.md -mtime -7 | grep -q ."
workflow: update-agents-md
`,
			wantID:     "agents-md-fresh",
			wantAuto:   "",
			wantChecks: 1,
		},
		{
			name: "multiple check steps",
			yaml: `
version: v0.1.0
id: full-check
auto: always
check:
  - shell: "test -d .argus/rules"
    description: "Rules dir exists"
  - shell: "test -f CLAUDE.md"
    description: "CLAUDE.md exists"
  - shell: "test -f .agents/skills/argus-doctor/SKILL.md"
    description: "Skills generated"
workflow: argus-init
`,
			wantID:     "full-check",
			wantAuto:   "always",
			wantChecks: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, err := ParseInvariant(strings.NewReader(tt.yaml))
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, inv.ID)
			assert.Equal(t, tt.wantAuto, inv.Auto)
			assert.Len(t, inv.Check, tt.wantChecks)
		})
	}
}

func TestParseInvariant_Errors(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantErr   error
		wantInMsg string
	}{
		{
			name: "missing version",
			yaml: `
id: my-check
check:
  - shell: "test -f README.md"
prompt: "Create a README"
`,
			wantInMsg: "version",
		},
		{
			name: "missing id",
			yaml: `
version: v0.1.0
check:
  - shell: "test -f README.md"
prompt: "Create a README"
`,
			wantErr:   core.ErrInvalidID,
			wantInMsg: "invariant ID",
		},
		{
			name: "invalid id uppercase",
			yaml: `
version: v0.1.0
id: MY-CHECK
check:
  - shell: "test -f README.md"
prompt: "Create a README"
`,
			wantErr: core.ErrInvalidID,
		},
		{
			name: "invalid auto value",
			yaml: `
version: v0.1.0
id: my-check
auto: daily
check:
  - shell: "test -f README.md"
prompt: "Create a README"
`,
			wantInMsg: "auto",
		},
		{
			name: "empty check list",
			yaml: `
version: v0.1.0
id: my-check
check: []
prompt: "Create a README"
`,
			wantInMsg: "at least one check step",
		},
		{
			name: "both prompt and workflow empty",
			yaml: `
version: v0.1.0
id: my-check
check:
  - shell: "test -f README.md"
`,
			wantInMsg: "prompt or workflow",
		},
		{
			name: "unknown yaml key",
			yaml: `
version: v0.1.0
id: my-check
unknown_field: "bad"
check:
  - shell: "test -f README.md"
prompt: "Create a README"
`,
			wantInMsg: "unknown_field",
		},
		{
			name: "incompatible version",
			yaml: `
version: v2.0.0
id: my-check
check:
  - shell: "test -f README.md"
prompt: "Create a README"
`,
			wantErr: core.ErrVersionMismatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseInvariant(strings.NewReader(tt.yaml))
			require.Error(t, err)
			if tt.wantErr != nil {
				assert.True(t, errors.Is(err, tt.wantErr),
					"expected %v, got: %v", tt.wantErr, err)
			}
			if tt.wantInMsg != "" {
				assert.Contains(t, err.Error(), tt.wantInMsg)
			}
		})
	}
}

func TestPromptWorkflow(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "both empty errors",
			yaml: `
version: v0.1.0
id: my-check
check:
  - shell: "test -f README.md"
`,
			wantErr: true,
		},
		{
			name: "only prompt ok",
			yaml: `
version: v0.1.0
id: my-check
check:
  - shell: "test -f README.md"
prompt: "Fix it"
`,
			wantErr: false,
		},
		{
			name: "only workflow ok",
			yaml: `
version: v0.1.0
id: my-check
check:
  - shell: "test -f README.md"
workflow: fix-it
`,
			wantErr: false,
		},
		{
			name: "both present ok",
			yaml: `
version: v0.1.0
id: my-check
check:
  - shell: "test -f README.md"
prompt: "Fix it"
workflow: fix-it
`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseInvariant(strings.NewReader(tt.yaml))
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "prompt or workflow")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
