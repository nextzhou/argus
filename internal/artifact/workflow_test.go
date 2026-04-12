package artifact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeWorkflowProviderFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

func TestWorkflowProviderLoad(t *testing.T) {
	tests := []struct {
		name         string
		workflowYAML string
		sharedYAML   string
		wantErr      string
		checkJobs    func(t *testing.T, jobs []workflow.Job)
	}{
		{
			name: "workflow with no refs returns unchanged",
			workflowYAML: `version: v0.1.0
id: no-refs
jobs:
  - id: build
    prompt: "Build the project"
  - id: test
    prompt: "Run tests"
`,
			checkJobs: func(t *testing.T, jobs []workflow.Job) {
				t.Helper()
				t.Helper()
				require.Len(t, jobs, 2)
				assert.Equal(t, "build", jobs[0].ID)
				assert.Equal(t, "Build the project", jobs[0].Prompt)
				assert.Empty(t, jobs[0].Ref)
				assert.Equal(t, "test", jobs[1].ID)
				assert.Equal(t, "Run tests", jobs[1].Prompt)
				assert.Empty(t, jobs[1].Ref)
			},
		},
		{
			name: "workflow with valid refs resolves successfully",
			workflowYAML: `version: v0.1.0
id: with-refs
jobs:
  - ref: build_job
  - id: custom_test
    ref: test_job
    prompt: "Custom test prompt"
`,
			sharedYAML: `jobs:
  build_job:
    id: build_job
    prompt: "Build from shared"
    skill: "argus-build"
  test_job:
    id: test_job
    prompt: "Test from shared"
    skill: "argus-test"
`,
			checkJobs: func(t *testing.T, jobs []workflow.Job) {
				t.Helper()
				t.Helper()
				require.Len(t, jobs, 2)
				assert.Equal(t, "build_job", jobs[0].ID)
				assert.Equal(t, "Build from shared", jobs[0].Prompt)
				assert.Equal(t, "argus-build", jobs[0].Skill)
				assert.Equal(t, "build_job", jobs[0].Ref)
				assert.Equal(t, "custom_test", jobs[1].ID)
				assert.Equal(t, "Custom test prompt", jobs[1].Prompt)
				assert.Equal(t, "argus-test", jobs[1].Skill)
				assert.Equal(t, "test_job", jobs[1].Ref)
			},
		},
		{
			name: "missing shared file returns error",
			workflowYAML: `version: v0.1.0
id: missing-shared
jobs:
  - ref: some_job
`,
			wantErr: "loading shared definitions",
		},
		{
			name: "missing referenced job returns error",
			workflowYAML: `version: v0.1.0
id: ref-not-found
jobs:
  - ref: nonexistent_job
`,
			sharedYAML: `jobs:
  existing_job:
    id: existing_job
    prompt: "Exists"
`,
			wantErr: "resolving ref for job[0]",
		},
		{
			name: "mixed direct and ref jobs resolve correctly",
			workflowYAML: `version: v0.1.0
id: mixed-jobs
jobs:
  - id: direct_job
    prompt: "Direct prompt"
  - ref: shared_job
  - id: another_direct
    prompt: "Another direct"
`,
			sharedYAML: `jobs:
  shared_job:
    id: shared_job
    prompt: "Shared prompt"
    skill: "argus-shared"
`,
			checkJobs: func(t *testing.T, jobs []workflow.Job) {
				t.Helper()
				t.Helper()
				require.Len(t, jobs, 3)
				assert.Equal(t, "direct_job", jobs[0].ID)
				assert.Equal(t, "Direct prompt", jobs[0].Prompt)
				assert.Empty(t, jobs[0].Ref)
				assert.Equal(t, "shared_job", jobs[1].ID)
				assert.Equal(t, "Shared prompt", jobs[1].Prompt)
				assert.Equal(t, "argus-shared", jobs[1].Skill)
				assert.Equal(t, "shared_job", jobs[1].Ref)
				assert.Equal(t, "another_direct", jobs[2].ID)
				assert.Equal(t, "Another direct", jobs[2].Prompt)
				assert.Empty(t, jobs[2].Ref)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeWorkflowProviderFile(t, dir, "test-wf.yaml", tt.workflowYAML)
			if tt.sharedYAML != "" {
				writeWorkflowProviderFile(t, dir, "_shared.yaml", tt.sharedYAML)
			}

			provider := NewWorkflowProvider("", dir)
			loaded, err := provider.Load("test-wf")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, loaded)
			if tt.checkJobs != nil {
				tt.checkJobs(t, loaded.Jobs)
			}
		})
	}
}
