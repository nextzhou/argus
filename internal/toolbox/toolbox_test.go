package toolbox

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunTouchTimestamp(t *testing.T) {
	t.Run("writes compact UTC timestamp to file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ts.txt")

		var stdout, stderr bytes.Buffer
		code := RunTouchTimestamp([]string{path}, &stdout, &stderr)

		assert.Equal(t, 0, code)
		assert.Empty(t, stderr.String())

		//nolint:gosec // The test reads a temp file path created under t.TempDir.
		content, err := os.ReadFile(path)
		require.NoError(t, err)

		// Verify format: YYYYMMDDTHHMMSSZ
		matched, err := regexp.MatchString(`^\d{8}T\d{6}Z$`, string(content))
		require.NoError(t, err)
		assert.True(t, matched, "timestamp %q should match YYYYMMDDTHHMMSSZ format", string(content))
	})

	t.Run("error when no args", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := RunTouchTimestamp(nil, &stdout, &stderr)

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr.String(), "usage")
	})

	t.Run("error when directory does not exist", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := RunTouchTimestamp([]string{"/nonexistent/dir/file.txt"}, &stdout, &stderr)

		assert.Equal(t, 1, code)
		assert.NotEmpty(t, stderr.String())
	})
}

func TestRunSHA256Sum(t *testing.T) {
	// echo -n "hello" | sha256sum -> 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824  -
	expectedHash := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

	t.Run("stdin mode with no args", func(t *testing.T) {
		stdin := strings.NewReader("hello")
		var stdout, stderr bytes.Buffer
		code := RunSHA256Sum(nil, stdin, &stdout, &stderr)

		assert.Equal(t, 0, code)
		assert.Empty(t, stderr.String())
		assert.Equal(t, expectedHash+"  -\n", stdout.String())
	})

	t.Run("stdin mode with dash arg", func(t *testing.T) {
		stdin := strings.NewReader("hello")
		var stdout, stderr bytes.Buffer
		code := RunSHA256Sum([]string{"-"}, stdin, &stdout, &stderr)

		assert.Equal(t, 0, code)
		assert.Equal(t, expectedHash+"  -\n", stdout.String())
	})

	t.Run("file mode", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))

		var stdout, stderr bytes.Buffer
		code := RunSHA256Sum([]string{path}, nil, &stdout, &stderr)

		assert.Equal(t, 0, code)
		assert.Empty(t, stderr.String())
		assert.Equal(t, expectedHash+"  "+path+"\n", stdout.String())
	})

	t.Run("error on nonexistent file", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := RunSHA256Sum([]string{"/nonexistent/file"}, nil, &stdout, &stderr)

		assert.Equal(t, 1, code)
		assert.NotEmpty(t, stderr.String())
	})
}

func TestRunJQ(t *testing.T) {
	t.Run("extract field from JSON", func(t *testing.T) {
		stdin := strings.NewReader(`{"count":42}`)
		var stdout, stderr bytes.Buffer
		code := RunJQ([]string{".count"}, stdin, &stdout, &stderr)

		assert.Equal(t, 0, code)
		assert.Empty(t, stderr.String())
		assert.Equal(t, "42\n", stdout.String())
	})

	t.Run("extract string field", func(t *testing.T) {
		stdin := strings.NewReader(`{"name":"argus"}`)
		var stdout, stderr bytes.Buffer
		code := RunJQ([]string{".name"}, stdin, &stdout, &stderr)

		assert.Equal(t, 0, code)
		assert.Equal(t, "\"argus\"\n", stdout.String())
	})

	t.Run("error on invalid JSON", func(t *testing.T) {
		stdin := strings.NewReader(`not json`)
		var stdout, stderr bytes.Buffer
		code := RunJQ([]string{".foo"}, stdin, &stdout, &stderr)

		assert.Equal(t, 1, code)
		assert.NotEmpty(t, stderr.String())
	})

	t.Run("error when no query provided", func(t *testing.T) {
		stdin := strings.NewReader(`{}`)
		var stdout, stderr bytes.Buffer
		code := RunJQ(nil, stdin, &stdout, &stderr)

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr.String(), "usage")
	})

	t.Run("array access", func(t *testing.T) {
		stdin := strings.NewReader(`{"items":[1,2,3]}`)
		var stdout, stderr bytes.Buffer
		code := RunJQ([]string{".items[1]"}, stdin, &stdout, &stderr)

		assert.Equal(t, 0, code)
		assert.Equal(t, "2\n", stdout.String())
	})
}

func TestRunYQ(t *testing.T) {
	t.Run("extract field from YAML", func(t *testing.T) {
		stdin := strings.NewReader("name: argus\n")
		var stdout, stderr bytes.Buffer
		code := RunYQ([]string{".name"}, stdin, &stdout, &stderr)

		assert.Equal(t, 0, code)
		assert.Empty(t, stderr.String())
		assert.Equal(t, "argus\n", strings.TrimRight(stdout.String(), " "))
	})

	t.Run("extract nested field", func(t *testing.T) {
		stdin := strings.NewReader("parent:\n  child: value\n")
		var stdout, stderr bytes.Buffer
		code := RunYQ([]string{".parent.child"}, stdin, &stdout, &stderr)

		assert.Equal(t, 0, code)
		assert.Equal(t, "value\n", strings.TrimRight(stdout.String(), " "))
	})

	t.Run("error when no query provided", func(t *testing.T) {
		stdin := strings.NewReader("name: argus\n")
		var stdout, stderr bytes.Buffer
		code := RunYQ(nil, stdin, &stdout, &stderr)

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr.String(), "usage")
	})
}
