package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLoadShared(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantKeys []string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid shared with multiple jobs",
			content: `jobs:
  lint:
    prompt: "Run lint checks"
    skill: "code-review"
  run_tests:
    prompt: "Execute test suite"
`,
			wantKeys: []string{"lint", "run_tests"},
		},
		{
			name: "valid shared with single job",
			content: `jobs:
  build:
    prompt: "Build the project"
`,
			wantKeys: []string{"build"},
		},
		{
			name: "invalid key with hyphen",
			content: `jobs:
  my-job:
    prompt: "Run something"
`,
			wantErr: true,
			errMsg:  "my-job",
		},
		{
			name: "unknown fields in job rejected",
			content: `jobs:
  lint:
    prompt: "Run lint"
    unknown_field: "bad"
`,
			wantErr: true,
			errMsg:  "unknown_field",
		},
		{
			name:    "missing jobs key",
			content: `version: v0.1.0`,
			wantErr: true,
		},
		{
			name:    "empty jobs map",
			content: `jobs: {}`,
			wantErr: true,
			errMsg:  "at least one",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "_shared.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))

			shared, err := LoadShared(path)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}
			require.NoError(t, err)
			assert.Len(t, shared, len(tt.wantKeys))
			for _, key := range tt.wantKeys {
				assert.Contains(t, shared, key)
				assert.NotNil(t, shared[key])
			}
		})
	}
}

func TestLoadShared_NonExistentFile(t *testing.T) {
	_, err := LoadShared("/nonexistent/path/_shared.yaml")
	require.Error(t, err)
}

func TestLoadShared_FieldValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "_shared.yaml")
	content := `jobs:
  lint:
    prompt: "Run lint checks"
    skill: "code-review"
    description: "Lint job"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	shared, err := LoadShared(path)
	require.NoError(t, err)

	job := shared["lint"]
	assert.Equal(t, "Run lint checks", job.Prompt)
	assert.Equal(t, "code-review", job.Skill)
	assert.Equal(t, "Lint job", job.Description)
}

func TestResolveRef(t *testing.T) {
	shared := SharedJobs{
		"lint": {
			Prompt:      "Run lint checks",
			Skill:       "my-skill",
			Description: "Lint description",
		},
		"run_tests": {
			Prompt: "Execute tests",
			Skill:  "test-skill",
		},
	}

	tests := []struct {
		name      string
		jobYAML   string
		wantJob   Job
		wantErr   bool
		wantErrIs error
		errMsg    string
	}{
		{
			name:    "inherit all from shared when only ref present",
			jobYAML: `ref: lint`,
			wantJob: Job{
				ID:          "lint",
				Prompt:      "Run lint checks",
				Skill:       "my-skill",
				Description: "Lint description",
				Ref:         "lint",
			},
		},
		{
			name: "override prompt, inherit others",
			jobYAML: `ref: lint
prompt: "Override prompt"`,
			wantJob: Job{
				ID:          "lint",
				Prompt:      "Override prompt",
				Skill:       "my-skill",
				Description: "Lint description",
				Ref:         "lint",
			},
		},
		{
			name: "override prompt with empty string clears it",
			jobYAML: `ref: lint
prompt: ""`,
			wantJob: Job{
				ID:          "lint",
				Prompt:      "",
				Skill:       "my-skill",
				Description: "Lint description",
				Ref:         "lint",
			},
		},
		{
			name: "id absent inherits ref key as id",
			jobYAML: `ref: run_tests
prompt: "Custom test"`,
			wantJob: Job{
				ID:     "run_tests",
				Prompt: "Custom test",
				Skill:  "test-skill",
				Ref:    "run_tests",
			},
		},
		{
			name: "id present with value uses overlay id",
			jobYAML: `ref: lint
id: custom_lint`,
			wantJob: Job{
				ID:          "custom_lint",
				Prompt:      "Run lint checks",
				Skill:       "my-skill",
				Description: "Lint description",
				Ref:         "lint",
			},
		},
		{
			name: "override all fields",
			jobYAML: `ref: lint
id: new_lint
prompt: "New prompt"
skill: "new-skill"
description: "New desc"`,
			wantJob: Job{
				ID:          "new_lint",
				Prompt:      "New prompt",
				Skill:       "new-skill",
				Description: "New desc",
				Ref:         "lint",
			},
		},
		{
			name:      "ref not found in shared",
			jobYAML:   `ref: nonexistent`,
			wantErr:   true,
			wantErrIs: core.ErrNotFound,
			errMsg:    "nonexistent",
		},
		{
			name:    "clear skill with null",
			jobYAML: "ref: lint\nskill: null",
			wantJob: Job{
				ID:          "lint",
				Prompt:      "Run lint checks",
				Skill:       "",
				Description: "Lint description",
				Ref:         "lint",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			err := yaml.Unmarshal([]byte(tt.jobYAML), &node)
			require.NoError(t, err, "failed to parse test YAML")
			// yaml.Unmarshal wraps in a document node
			require.Equal(t, yaml.DocumentNode, node.Kind)
			mappingNode := node.Content[0]

			job, err := ResolveRef(mappingNode, shared)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs,
						"expected %v, got: %v", tt.wantErrIs, err)
				}
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantJob, *job)
		})
	}
}

func TestResolveRef_NoRefKey(t *testing.T) {
	shared := SharedJobs{
		"lint": {Prompt: "Run lint"},
	}

	jobYAML := `prompt: "No ref here"`
	var node yaml.Node
	err := yaml.Unmarshal([]byte(jobYAML), &node)
	require.NoError(t, err)
	mappingNode := node.Content[0]

	_, err = ResolveRef(mappingNode, shared)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ref")
}
