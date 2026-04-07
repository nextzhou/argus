package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizePath(t *testing.T) {
	home := os.Getenv("HOME")
	require.NotEmpty(t, home, "HOME must be set for these tests")

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "tilde path with trailing slash",
			input: "~/work/company/",
			want:  "~/work/company",
		},
		{
			name:  "absolute path under HOME",
			input: home + "/work/company",
			want:  "~/work/company",
		},
		{
			name:  "absolute path outside HOME",
			input: "/usr/local/bin",
			want:  "/usr/local/bin",
		},
		{
			name:  "tilde path with dot-dot",
			input: "~/work/company/../client",
			want:  "~/work/client",
		},
		{
			name:  "bare tilde",
			input: "~",
			want:  "~",
		},
		{
			name:  "HOME itself as absolute",
			input: home,
			want:  "~",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePath(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizePath_RelativePath(t *testing.T) {
	home := os.Getenv("HOME")
	require.NotEmpty(t, home)

	cwd, err := os.Getwd()
	require.NoError(t, err)

	got, err := NormalizePath(".")
	require.NoError(t, err)

	expected := filepath.Clean(cwd)
	if expected == home {
		expected = "~"
	} else if len(expected) > len(home) && expected[:len(home)] == home && expected[len(home)] == filepath.Separator {
		expected = "~" + expected[len(home):]
	}
	assert.Equal(t, expected, got)
}

func TestNormalizePath_DotDotRelative(t *testing.T) {
	home := os.Getenv("HOME")
	require.NotEmpty(t, home)

	cwd, err := os.Getwd()
	require.NoError(t, err)

	got, err := NormalizePath("../")
	require.NoError(t, err)

	parent := filepath.Clean(filepath.Dir(cwd))
	expected := parent
	if expected == home {
		expected = "~"
	} else if len(expected) > len(home) && expected[:len(home)] == home && expected[len(home)] == filepath.Separator {
		expected = "~" + expected[len(home):]
	}
	assert.Equal(t, expected, got)
}

func TestExpandPath(t *testing.T) {
	home := os.Getenv("HOME")
	require.NotEmpty(t, home)

	tests := []struct {
		name   string
		stored string
		want   string
	}{
		{
			name:   "tilde expansion",
			stored: "~/work/company",
			want:   home + "/work/company",
		},
		{
			name:   "bare tilde",
			stored: "~",
			want:   home,
		},
		{
			name:   "absolute unchanged",
			stored: "/usr/local/bin",
			want:   "/usr/local/bin",
		},
		{
			name:   "no tilde prefix unchanged",
			stored: "relative/path",
			want:   "relative/path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandPath(tt.stored)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsInWorkspace(t *testing.T) {
	home := os.Getenv("HOME")
	require.NotEmpty(t, home)

	tests := []struct {
		name        string
		projectPath string
		workspaces  []string
		want        bool
	}{
		{
			name:        "project inside workspace",
			projectPath: home + "/work/company/argus",
			workspaces:  []string{"~/work/company"},
			want:        true,
		},
		{
			name:        "exact match counts as inside",
			projectPath: home + "/work/company",
			workspaces:  []string{"~/work/company"},
			want:        true,
		},
		{
			name:        "false positive prevention - prefix substring",
			projectPath: home + "/work/co",
			workspaces:  []string{"~/work/company"},
			want:        false,
		},
		{
			name:        "false positive prevention - longer prefix",
			projectPath: home + "/work/company-extra/proj",
			workspaces:  []string{"~/work/company"},
			want:        false,
		},
		{
			name:        "no match - different tree",
			projectPath: home + "/personal/proj",
			workspaces:  []string{"~/work/company"},
			want:        false,
		},
		{
			name:        "multiple workspaces - second matches",
			projectPath: home + "/work/client-x/proj",
			workspaces:  []string{"~/work/company", "~/work/client-x"},
			want:        true,
		},
		{
			name:        "empty workspaces",
			projectPath: home + "/work/company/argus",
			workspaces:  nil,
			want:        false,
		},
		{
			name:        "deeply nested project",
			projectPath: home + "/work/company/team/project/subdir",
			workspaces:  []string{"~/work/company"},
			want:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsInWorkspace(tt.projectPath, tt.workspaces)
			assert.Equal(t, tt.want, got)
		})
	}
}
