package workflow

import (
	"strings"
	"testing"

	"github.com/nextzhou/argus/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkflow(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantID   string
		wantJobs int
	}{
		{
			name: "minimal valid workflow",
			yaml: `
version: v0.1.0
id: release
jobs:
  - prompt: "Run go test ./... and report results"
`,
			wantID:   "release",
			wantJobs: 1,
		},
		{
			name: "full workflow with all fields",
			yaml: `
version: v0.1.0
id: release-flow
description: "Standard release flow"
jobs:
  - id: run_tests
    description: "Run the test suite"
    prompt: "Run go test ./... and report results"
    skill: argus-run-tests
  - id: lint_check
    description: "Check linting"
    prompt: "Run make lint"
    skill: argus-run-lint
`,
			wantID:   "release-flow",
			wantJobs: 2,
		},
		{
			name: "job with ref field",
			yaml: `
version: v0.1.0
id: my-workflow
jobs:
  - id: shared_step
    ref: "shared.run_tests"
`,
			wantID:   "my-workflow",
			wantJobs: 1,
		},
		{
			name: "job with only skill",
			yaml: `
version: v0.1.0
id: lint-only
jobs:
  - skill: argus-run-lint
`,
			wantID:   "lint-only",
			wantJobs: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := ParseWorkflow(strings.NewReader(tt.yaml))
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, w.ID)
			assert.Len(t, w.Jobs, tt.wantJobs)
		})
	}
}

func TestParseWorkflow_Errors(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantErr   error
		wantInMsg string
	}{
		{
			name: "missing version",
			yaml: `
id: release
jobs:
  - prompt: "do something"
`,
			wantInMsg: "version",
		},
		{
			name: "missing id",
			yaml: `
version: v0.1.0
jobs:
  - prompt: "do something"
`,
			wantErr:   core.ErrInvalidID,
			wantInMsg: "workflow ID",
		},
		{
			name: "invalid workflow id uppercase",
			yaml: `
version: v0.1.0
id: MY-WORKFLOW
jobs:
  - prompt: "do something"
`,
			wantErr: core.ErrInvalidID,
		},
		{
			name: "invalid job id with hyphen",
			yaml: `
version: v0.1.0
id: release
jobs:
  - id: my-job
    prompt: "do something"
`,
			wantErr: core.ErrInvalidID,
		},
		{
			name: "empty jobs list",
			yaml: `
version: v0.1.0
id: release
jobs: []
`,
			wantInMsg: "at least one job",
		},
		{
			name: "non-ref job missing both prompt and skill",
			yaml: `
version: v0.1.0
id: release
jobs:
  - id: empty_job
    description: "no prompt or skill"
`,
			wantInMsg: "prompt or skill",
		},
		{
			name: "unknown yaml key",
			yaml: `
version: v0.1.0
id: release
unknown_field: "bad"
jobs:
  - prompt: "do something"
`,
			wantInMsg: "unknown_field",
		},
		{
			name: "incompatible version",
			yaml: `
version: v2.0.0
id: release
jobs:
  - prompt: "do something"
`,
			wantErr: core.ErrVersionMismatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseWorkflow(strings.NewReader(tt.yaml))
			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr,
					"expected %v, got: %v", tt.wantErr, err)
			}
			if tt.wantInMsg != "" {
				assert.Contains(t, err.Error(), tt.wantInMsg)
			}
		})
	}
}
