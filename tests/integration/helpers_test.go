package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// binaryPath holds the path to the compiled argus binary, set by TestMain.
var binaryPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "argus-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
		os.Exit(1)
	}

	binaryPath = filepath.Join(tmpDir, "argus")

	projectRoot := findProjectRoot()
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/argus")
	buildCmd.Dir = projectRoot
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building argus binary: %v\n%s\n", err, output)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

// findProjectRoot walks up from the current directory to find the project root
// (directory containing go.mod).
func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getting cwd: %v\n", err)
		os.Exit(1)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Fprintf(os.Stderr, "could not find project root (go.mod)\n")
			os.Exit(1)
		}
		dir = parent
	}
}

// cmdResult holds the output and exit code from running the argus binary.
type cmdResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// runArgus executes the argus binary with the given args in the current working directory.
// The caller must have set the working directory (via t.Chdir or workDir param).
func runArgus(t *testing.T, workDir string, args ...string) cmdResult {
	t.Helper()
	return runArgusWithStdin(t, workDir, "", args...)
}

// runArgusWithStdin executes the argus binary with stdin content and given args.
func runArgusWithStdin(t *testing.T, workDir string, stdin string, args ...string) cmdResult {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = workDir
	cmd.Env = os.Environ()

	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("running argus %v: %v", args, err)
		}
	}

	return cmdResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// setupGitRepo creates a temporary directory with a .git directory to simulate a git repo.
// Returns the real (symlink-resolved) absolute path to the directory.
// Symlink resolution is needed because macOS /var/folders is a symlink to /private/var/folders,
// and path comparisons between parent process paths and child process os.Getwd() would mismatch.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(resolved, ".git"), 0o755))
	return resolved
}

// resolveSymlinks resolves any symlinks in the given path.
// Required on macOS where /var/folders symlinks to /private/var/folders.
func resolveSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}

// writeFile writes content to a file at dir/relPath, creating parent directories as needed.
func writeFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
}

// parseJSON parses stdout as JSON into a map. Fails the test if parsing fails.
func parseJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var data map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data),
		"output should be valid JSON: %s", stdout)
	return data
}

// requireOK asserts the JSON output has status "ok" and returns the parsed data.
func requireOK(t *testing.T, result cmdResult) map[string]any {
	t.Helper()
	require.Equal(t, 0, result.ExitCode, "expected exit code 0, stderr: %s", result.Stderr)
	data := parseJSON(t, result.Stdout)
	require.Equal(t, "ok", data["status"], "expected ok status, got: %s", result.Stdout)
	return data
}

// requireError asserts the command exited non-zero with an error envelope and returns the parsed data.
// The error envelope format is: {"status":"error","message":"..."}.
func requireError(t *testing.T, result cmdResult) map[string]any {
	t.Helper()
	require.NotEqual(t, 0, result.ExitCode, "expected non-zero exit code")
	data := parseJSON(t, result.Stdout)
	require.Equal(t, "error", data["status"], "expected error status, got: %s", result.Stdout)
	_, hasMessage := data["message"]
	require.True(t, hasMessage, "error envelope must contain 'message' field")
	return data
}

// fileExists checks whether a file or directory exists at the given path.
func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	require.NoError(t, err)
	return true
}
