package hook

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalTick(t *testing.T) {
	const guidance = `[Argus] This project is inside a registered workspace but Argus is not installed.

To install Argus in this project, run:
  argus install --yes

For guidance, use the argus-install Skill.
`

	tests := []struct {
		name            string
		stdin           string
		setup           func(t *testing.T, homeDir string) string
		want            string
		wantErrContains string
	}{
		{
			name:  "sub agent",
			stdin: `{"session_id":"ses-sub-agent","agent_id":"worker-1"}`,
			setup: func(t *testing.T, _ string) string {
				t.Helper()
				return t.TempDir()
			},
		},
		{
			name:  "malformed input",
			stdin: `{"session_id":`,
			setup: func(t *testing.T, _ string) string {
				t.Helper()
				return t.TempDir()
			},
		},
		{
			name:  "non git dir",
			stdin: `{"session_id":"ses-non-git"}`,
			setup: func(t *testing.T, _ string) string {
				t.Helper()
				return t.TempDir()
			},
		},
		{
			name:  "installed project",
			stdin: `{"session_id":"ses-installed"}`,
			setup: func(t *testing.T, homeDir string) string {
				t.Helper()
				projectDir := filepath.Join(homeDir, "installed-project")
				createGitProject(t, projectDir, true)
				return projectDir
			},
		},
		{
			name:  "not in workspace",
			stdin: `{"session_id":"ses-not-in-workspace"}`,
			setup: func(t *testing.T, homeDir string) string {
				t.Helper()

				workspaceDir := filepath.Join(homeDir, "company-workspace")
				storedWorkspace := normalizeWorkspacePath(t, workspaceDir)
				writeWorkspaceConfig(t, homeDir, &workspace.Config{Workspaces: []string{storedWorkspace}})

				projectDir := filepath.Join(homeDir, "personal-project")
				createGitProject(t, projectDir, false)
				return projectDir
			},
		},
		{
			name:  "in workspace uninstalled",
			stdin: `{"session_id":"ses-workspace"}`,
			setup: func(t *testing.T, homeDir string) string {
				t.Helper()

				workspaceDir := filepath.Join(homeDir, "work", "company")
				storedWorkspace := normalizeWorkspacePath(t, workspaceDir)
				writeWorkspaceConfig(t, homeDir, &workspace.Config{Workspaces: []string{storedWorkspace}})

				projectDir := filepath.Join(workspaceDir, "argus")
				createGitProject(t, projectDir, false)
				return projectDir
			},
			want: guidance,
		},
		{
			name:  "no workspace config",
			stdin: `{"session_id":"ses-no-config"}`,
			setup: func(t *testing.T, homeDir string) string {
				t.Helper()
				projectDir := filepath.Join(homeDir, "no-config-project")
				createGitProject(t, projectDir, false)
				return projectDir
			},
		},
		{
			name:  "empty workspace list",
			stdin: `{"session_id":"ses-empty-workspaces"}`,
			setup: func(t *testing.T, homeDir string) string {
				t.Helper()

				writeWorkspaceConfig(t, homeDir, &workspace.Config{Workspaces: []string{}})

				projectDir := filepath.Join(homeDir, "empty-workspaces-project")
				createGitProject(t, projectDir, false)
				return projectDir
			},
		},
		{
			name:  "invalid workspace config",
			stdin: `{"session_id":"ses-invalid-config"}`,
			setup: func(t *testing.T, homeDir string) string {
				t.Helper()

				writeRawWorkspaceConfig(t, homeDir, "{{invalid yaml")

				projectDir := filepath.Join(homeDir, "invalid-config-project")
				createGitProject(t, projectDir, false)
				return projectDir
			},
			wantErrContains: "loading workspace config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)

			cwd := tt.setup(t, homeDir)
			got, err := HandleGlobalTick(cwd, bytes.NewBufferString(tt.stdin), "claude-code")

			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func createGitProject(t *testing.T, projectDir string, installed bool) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".git"), 0o755))
	if installed {
		require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".argus"), 0o755))
	}
}

func normalizeWorkspacePath(t *testing.T, workspaceDir string) string {
	t.Helper()
	normalized, err := workspace.NormalizePath(workspaceDir)
	require.NoError(t, err)
	return normalized
}

func writeWorkspaceConfig(t *testing.T, homeDir string, cfg *workspace.Config) {
	t.Helper()
	require.NoError(t, workspace.SaveConfig(filepath.Join(homeDir, ".config", "argus", "config.yaml"), cfg))
}

func writeRawWorkspaceConfig(t *testing.T, homeDir, content string) {
	t.Helper()
	configPath := filepath.Join(homeDir, ".config", "argus", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0o644))
}
