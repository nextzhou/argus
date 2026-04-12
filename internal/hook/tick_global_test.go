package hook

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/assets"
	"github.com/nextzhou/argus/internal/session"
	"github.com/nextzhou/argus/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleTick_Global(t *testing.T) {
	tests := []struct {
		name   string
		stdin  string
		setup  func(t *testing.T, homeDir string) string
		assert func(t *testing.T, output string, sessionBaseDir string)
	}{
		{
			name:  "sub agent returns empty output",
			stdin: `{"session_id":"ses-sub-agent","agent_id":"worker-1"}`,
			setup: func(t *testing.T, _ string) string {
				t.Helper()
				t.Helper()
				return t.TempDir()
			},
			assert: func(t *testing.T, output string, _ string) {
				t.Helper()
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name:  "non git directory returns empty output",
			stdin: `{"session_id":"ses-non-git"}`,
			setup: func(t *testing.T, _ string) string {
				t.Helper()
				t.Helper()
				return t.TempDir()
			},
			assert: func(t *testing.T, output string, _ string) {
				t.Helper()
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name:  "set up project uses project scope in global mode",
			stdin: `{"session_id":"ses-set-up"}`,
			setup: func(t *testing.T, homeDir string) string {
				t.Helper()
				t.Helper()
				projectDir := filepath.Join(homeDir, "set-up-project")
				createTickGlobalProject(t, projectDir, true)
				writeTickGlobalWorkspaceConfig(t, homeDir, []string{normalizeTickGlobalWorkspacePath(t, homeDir)})
				writeTickWorkflowFixture(t, projectDir, "argus-project-init", `version: v0.1.0
id: argus-project-init
description: "Complete project-specific Argus initialization"
jobs:
  - id: generate_rules
    prompt: "Generate project rules"
`)
				return projectDir
			},
			assert: func(t *testing.T, output string, sessionBaseDir string) {
				t.Helper()
				t.Helper()
				assertHookSafeTickText(t, output)
				assert.Contains(t, output, "No active pipeline")
				assert.Contains(t, output, "argus-project-init")
				assert.NotContains(t, output, "argus-project-setup")
				assert.True(t, session.Exists(sessionBaseDir, "ses-set-up"))
			},
		},
		{
			name:  "workspace project without argus uses invariant output",
			stdin: `{"session_id":"ses-workspace"}`,
			setup: func(t *testing.T, homeDir string) string {
				t.Helper()
				t.Helper()
				workspaceDir := filepath.Join(homeDir, "work", "company")
				writeTickGlobalWorkspaceConfig(t, homeDir, []string{normalizeTickGlobalWorkspacePath(t, workspaceDir)})
				writeTickGlobalInvariant(t, homeDir)

				projectDir := filepath.Join(workspaceDir, "argus")
				createTickGlobalProject(t, projectDir, false)
				return projectDir
			},
			assert: func(t *testing.T, output string, sessionBaseDir string) {
				t.Helper()
				t.Helper()
				assertHookSafeTickText(t, output)
				assert.Contains(t, output, "Argus: Invariant check failed:")
				assert.Contains(t, output, "argus-project-setup")
				assert.Contains(t, output, "Project-level Argus setup exists")
				assert.Contains(t, output, "question tool")
				assert.Contains(t, output, "argus-setup")
				assert.Contains(t, output, "argus-intro")
				assert.NotContains(t, output, "No active pipeline")
				assert.True(t, session.Exists(sessionBaseDir, "ses-workspace"))
			},
		},
		{
			name:  "project outside workspace returns empty output",
			stdin: `{"session_id":"ses-outside-workspace"}`,
			setup: func(t *testing.T, homeDir string) string {
				t.Helper()
				t.Helper()
				workspaceDir := filepath.Join(homeDir, "work", "company")
				writeTickGlobalWorkspaceConfig(t, homeDir, []string{normalizeTickGlobalWorkspacePath(t, workspaceDir)})

				projectDir := filepath.Join(homeDir, "personal-project")
				createTickGlobalProject(t, projectDir, false)
				return projectDir
			},
			assert: func(t *testing.T, output string, _ string) {
				t.Helper()
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name:  "missing workspace config fails open",
			stdin: `{"session_id":"ses-missing-config"}`,
			setup: func(t *testing.T, homeDir string) string {
				t.Helper()
				t.Helper()
				projectDir := filepath.Join(homeDir, "missing-config-project")
				createTickGlobalProject(t, projectDir, false)
				return projectDir
			},
			assert: func(t *testing.T, output string, _ string) {
				t.Helper()
				t.Helper()
				assert.Empty(t, output)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)

			projectDir := tt.setup(t, homeDir)
			sessionBaseDir := t.TempDir()

			var out bytes.Buffer
			err := HandleTick(context.Background(), "claude-code", true, bytes.NewBufferString(tt.stdin), &out, projectDir, sessionBaseDir)
			require.NoError(t, err)

			tt.assert(t, out.String(), sessionBaseDir)
		})
	}
}

func createTickGlobalProject(t *testing.T, projectDir string, isSetUp bool) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".git"), 0o700))
	if isSetUp {
		require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".argus"), 0o700))
	}
}

func normalizeTickGlobalWorkspacePath(t *testing.T, workspaceDir string) string {
	t.Helper()
	normalized, err := workspace.NormalizePath(workspaceDir)
	require.NoError(t, err)
	return normalized
}

func writeTickGlobalWorkspaceConfig(t *testing.T, homeDir string, workspaces []string) {
	t.Helper()
	require.NoError(t, workspace.SaveConfig(filepath.Join(homeDir, ".config", "argus", "config.yaml"), &workspace.Config{Workspaces: workspaces}))
}

func writeTickGlobalInvariant(t *testing.T, homeDir string) {
	t.Helper()
	data, err := assets.ReadAsset("invariants/argus-project-setup.yaml")
	require.NoError(t, err)

	path := filepath.Join(homeDir, ".config", "argus", "invariants", "argus-project-setup.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, data, 0o600))
}
