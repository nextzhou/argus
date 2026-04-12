package scope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveScopeForTick(t *testing.T) {
	tests := []struct {
		name            string
		global          bool
		setup           func(t *testing.T, homeDir string) *workspace.ProjectRoot
		wantScope       string
		wantErrContains string
	}{
		{
			name:   "project tick uses project scope for set-up project",
			global: false,
			setup: func(t *testing.T, homeDir string) *workspace.ProjectRoot {
				t.Helper()
				t.Helper()
				projectDir := filepath.Join(homeDir, "project-set-up")
				createResolveTestProject(t, projectDir, true)
				return &workspace.ProjectRoot{Path: projectDir, HasArgus: true, HasGit: true}
			},
			wantScope: "project",
		},
		{
			name:   "project tick skips project without setup",
			global: false,
			setup: func(t *testing.T, homeDir string) *workspace.ProjectRoot {
				t.Helper()
				t.Helper()
				projectDir := filepath.Join(homeDir, "project-not-set-up")
				createResolveTestProject(t, projectDir, false)
				return &workspace.ProjectRoot{Path: projectDir, HasArgus: false, HasGit: true}
			},
			wantScope: "nil",
		},
		{
			name:   "global tick resolves to project scope when repository is set up",
			global: true,
			setup: func(t *testing.T, homeDir string) *workspace.ProjectRoot {
				t.Helper()
				t.Helper()
				workspaceDir := filepath.Join(homeDir, "work", "company")
				writeResolveWorkspaceConfig(t, homeDir, []string{normalizeResolveWorkspacePath(t, workspaceDir)})
				projectDir := filepath.Join(workspaceDir, "set-up-project")
				createResolveTestProject(t, projectDir, true)
				return &workspace.ProjectRoot{Path: projectDir, HasArgus: true, HasGit: true}
			},
			wantScope: "project",
		},
		{
			name:   "global tick uses global scope for workspace member",
			global: true,
			setup: func(t *testing.T, homeDir string) *workspace.ProjectRoot {
				t.Helper()
				t.Helper()
				workspaceDir := filepath.Join(homeDir, "work", "company")
				writeResolveWorkspaceConfig(t, homeDir, []string{normalizeResolveWorkspacePath(t, workspaceDir)})
				projectDir := filepath.Join(workspaceDir, "argus")
				createResolveTestProject(t, projectDir, false)
				return &workspace.ProjectRoot{Path: projectDir, HasArgus: false, HasGit: true}
			},
			wantScope: "global",
		},
		{
			name:   "global tick skips project outside workspace",
			global: true,
			setup: func(t *testing.T, homeDir string) *workspace.ProjectRoot {
				t.Helper()
				t.Helper()
				workspaceDir := filepath.Join(homeDir, "work", "company")
				writeResolveWorkspaceConfig(t, homeDir, []string{normalizeResolveWorkspacePath(t, workspaceDir)})
				projectDir := filepath.Join(homeDir, "personal-project")
				createResolveTestProject(t, projectDir, false)
				return &workspace.ProjectRoot{Path: projectDir, HasArgus: false, HasGit: true}
			},
			wantScope: "nil",
		},
		{
			name:   "global tick fails open when workspace config is missing",
			global: true,
			setup: func(t *testing.T, homeDir string) *workspace.ProjectRoot {
				t.Helper()
				t.Helper()
				projectDir := filepath.Join(homeDir, "missing-config-project")
				createResolveTestProject(t, projectDir, false)
				return &workspace.ProjectRoot{Path: projectDir, HasArgus: false, HasGit: true}
			},
			wantScope: "nil",
		},
		{
			name:   "global tick returns error for invalid workspace config",
			global: true,
			setup: func(t *testing.T, homeDir string) *workspace.ProjectRoot {
				t.Helper()
				t.Helper()
				projectDir := filepath.Join(homeDir, "invalid-config-project")
				createResolveTestProject(t, projectDir, false)
				writeResolveRawWorkspaceConfig(t, homeDir, "{{invalid yaml")
				return &workspace.ProjectRoot{Path: projectDir, HasArgus: false, HasGit: true}
			},
			wantErrContains: "loading workspace config",
		},
		{
			name:   "global tick skips when workspace list is empty",
			global: true,
			setup: func(t *testing.T, homeDir string) *workspace.ProjectRoot {
				t.Helper()
				t.Helper()
				writeResolveWorkspaceConfig(t, homeDir, []string{})
				projectDir := filepath.Join(homeDir, "empty-workspaces-project")
				createResolveTestProject(t, projectDir, false)
				return &workspace.ProjectRoot{Path: projectDir, HasArgus: false, HasGit: true}
			},
			wantScope: "nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)

			root := tt.setup(t, homeDir)
			resolved, err := ResolveScopeForTick(root, tt.global)

			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
			assertResolvedScope(t, resolved, tt.wantScope, homeDir, root.Path)
		})
	}
}

func TestResolveScope(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, homeDir string) resolveScopeTestInput
		wantScope string
	}{
		{
			name: "project with argus uses project scope",
			setup: func(t *testing.T, homeDir string) resolveScopeTestInput {
				t.Helper()
				t.Helper()
				projectDir := filepath.Join(homeDir, "project-set-up")
				createResolveTestProject(t, projectDir, true)
				cwd := filepath.Join(projectDir, "nested", "dir")
				require.NoError(t, os.MkdirAll(cwd, 0o700))
				return resolveScopeTestInput{cwd: cwd, projectDir: projectDir}
			},
			wantScope: "project",
		},
		{
			name: "project without argus in workspace uses global scope",
			setup: func(t *testing.T, homeDir string) resolveScopeTestInput {
				t.Helper()
				t.Helper()
				workspaceDir := filepath.Join(homeDir, "work", "company")
				writeResolveWorkspaceConfig(t, homeDir, []string{normalizeResolveWorkspacePath(t, workspaceDir)})
				projectDir := filepath.Join(workspaceDir, "argus")
				createResolveTestProject(t, projectDir, false)
				cwd := filepath.Join(projectDir, "nested")
				require.NoError(t, os.MkdirAll(cwd, 0o700))
				return resolveScopeTestInput{cwd: cwd, projectDir: projectDir}
			},
			wantScope: "global",
		},
		{
			name: "project without argus outside workspace returns nil",
			setup: func(t *testing.T, homeDir string) resolveScopeTestInput {
				t.Helper()
				t.Helper()
				workspaceDir := filepath.Join(homeDir, "work", "company")
				writeResolveWorkspaceConfig(t, homeDir, []string{normalizeResolveWorkspacePath(t, workspaceDir)})
				projectDir := filepath.Join(homeDir, "personal-project")
				createResolveTestProject(t, projectDir, false)
				cwd := filepath.Join(projectDir, "nested")
				require.NoError(t, os.MkdirAll(cwd, 0o700))
				return resolveScopeTestInput{cwd: cwd, projectDir: projectDir}
			},
			wantScope: "nil",
		},
		{
			name: "no project root returns nil",
			setup: func(t *testing.T, homeDir string) resolveScopeTestInput {
				t.Helper()
				t.Helper()
				cwd := filepath.Join(homeDir, "not-a-project")
				require.NoError(t, os.MkdirAll(cwd, 0o700))
				return resolveScopeTestInput{cwd: cwd}
			},
			wantScope: "nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)

			input := tt.setup(t, homeDir)
			resolved, err := ResolveScope(input.cwd)
			require.NoError(t, err)
			assertResolvedScope(t, resolved, tt.wantScope, homeDir, input.projectDir)
		})
	}
}

type resolveScopeTestInput struct {
	cwd        string
	projectDir string
}

func createResolveTestProject(t *testing.T, projectDir string, isSetUp bool) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".git"), 0o700))
	if isSetUp {
		require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".argus"), 0o700))
	}
}

func normalizeResolveWorkspacePath(t *testing.T, workspaceDir string) string {
	t.Helper()
	normalized, err := workspace.NormalizePath(workspaceDir)
	require.NoError(t, err)
	return normalized
}

func writeResolveWorkspaceConfig(t *testing.T, homeDir string, workspaces []string) {
	t.Helper()
	require.NoError(t, workspace.SaveConfig(filepath.Join(homeDir, ".config", "argus", "config.yaml"), &workspace.Config{Workspaces: workspaces}))
}

func writeResolveRawWorkspaceConfig(t *testing.T, homeDir, content string) {
	t.Helper()
	configPath := filepath.Join(homeDir, ".config", "argus", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o700))
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0o600))
}

func assertResolvedScope(t *testing.T, resolved *Resolved, wantScope, _ string, projectDir string) {
	t.Helper()

	switch wantScope {
	case "nil":
		assert.Nil(t, resolved)
	case "project":
		require.NotNil(t, resolved)
		assert.Equal(t, KindProject, resolved.Kind())
		assert.Equal(t, projectDir, resolved.ProjectRoot())
		require.NotNil(t, resolved.Artifacts())
	case "global":
		require.NotNil(t, resolved)
		assert.Equal(t, KindGlobal, resolved.Kind())
		assert.Equal(t, projectDir, resolved.ProjectRoot())
		require.NotNil(t, resolved.Artifacts())
	default:
		t.Fatalf("unknown scope expectation %q", wantScope)
	}
}
