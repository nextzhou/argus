package hook

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTemplateFullCoverage verifies every .tmpl file under internal/assets/prompts/
// is referenced in at least one non-test Go file under internal/hook/.
// This prevents orphaned templates from accumulating unnoticed.
func TestTemplateFullCoverage(t *testing.T) {
	promptsDir := filepath.Join("..", "assets", "prompts")
	entries, err := os.ReadDir(promptsDir)
	require.NoError(t, err, "reading prompts directory")

	var templates []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tmpl") {
			templates = append(templates, e.Name())
		}
	}
	require.NotEmpty(t, templates, "no .tmpl files found; directory layout may have changed")

	hookEntries, err := os.ReadDir(".")
	require.NoError(t, err, "reading hook directory")

	var hookSource strings.Builder
	for _, e := range hookEntries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		//nolint:gosec // The test reads Go source files enumerated from the current package directory.
		data, readErr := os.ReadFile(name)
		require.NoError(t, readErr, "reading hook source file %s", name)
		hookSource.Write(data)
	}

	combined := hookSource.String()
	for _, tmpl := range templates {
		// Templates are referenced as "prompts/<filename>" in Go code.
		ref := "prompts/" + tmpl
		assert.Contains(t, combined, ref,
			"template %q is not referenced in any non-test Go file under internal/hook/", tmpl)
	}
}

// TestGoVersionConsistency verifies that the Go version declared in go.mod
// and .golangci.yml agree on major.minor, preventing silent version drift
// between the build toolchain and the linter.
func TestGoVersionConsistency(t *testing.T) {
	goModPath := filepath.Join("..", "..", "go.mod")
	golangciPath := filepath.Join("..", "..", ".golangci.yml")

	//nolint:gosec // The test reads repository metadata files at fixed relative paths.
	goModData, err := os.ReadFile(goModPath)
	require.NoError(t, err, "reading go.mod")

	//nolint:gosec // The test reads repository metadata files at fixed relative paths.
	golangciData, err := os.ReadFile(golangciPath)
	require.NoError(t, err, "reading .golangci.yml")

	// Extract major.minor from go.mod's "go X.Y" directive.
	goModRe := regexp.MustCompile(`(?m)^go\s+(\d+\.\d+)`)
	goModMatch := goModRe.FindSubmatch(goModData)
	require.NotNil(t, goModMatch, "go.mod does not contain a recognizable 'go X.Y' directive")
	goModVersion := string(goModMatch[1])

	// Extract major.minor from .golangci.yml's "go:" field.
	golangciRe := regexp.MustCompile(`(?m)^\s+go:\s+"?(\d+\.\d+)"?`)
	golangciMatch := golangciRe.FindSubmatch(golangciData)
	require.NotNil(t, golangciMatch, ".golangci.yml does not contain a recognizable 'go: X.Y' field")
	golangciVersion := string(golangciMatch[1])

	assert.Equal(t, goModVersion, golangciVersion,
		"go.mod (go %s) and .golangci.yml (go: %s) major.minor versions must match",
		goModVersion, golangciVersion)
}

// TestValidatePathPersistenceCoverage verifies that every function performing
// file I/O in persistence store files also calls core.ValidatePath, preventing
// path traversal vulnerabilities from creeping in via new store functions.
func TestValidatePathPersistenceCoverage(t *testing.T) {
	storeFiles := []string{
		filepath.Join("..", "pipeline", "store.go"),
		filepath.Join("..", "session", "store.go"),
	}

	// I/O operations that indicate a function touches the filesystem with a
	// user-influenced path. os.ReadDir is excluded because ScanActivePipelines
	// delegates to LoadPipeline for per-file validation.
	ioPattern := regexp.MustCompile(`os\.(Open|ReadFile|WriteFile|Create|Stat)\b`)

	// Split source into top-level function blocks. The regex captures each
	// "func ... {" through the next "^}" (top-level closing brace).
	funcPattern := regexp.MustCompile(`(?ms)^func\s+\w[^\n]*\{.*?^}`)

	for _, path := range storeFiles {
		//nolint:gosec // The test reads store implementation files from a fixed package-local allowlist.
		data, err := os.ReadFile(path)
		require.NoError(t, err, "reading store file %s", path)

		funcs := funcPattern.FindAll(data, -1)
		require.NotEmpty(t, funcs, "no functions found in %s; regex may need updating", path)

		for _, fn := range funcs {
			fnStr := string(fn)
			if !ioPattern.MatchString(fnStr) {
				continue
			}

			nameRe := regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?(\w+)`)
			nameMatch := nameRe.FindStringSubmatch(fnStr)
			funcName := "<unknown>"
			if nameMatch != nil {
				funcName = nameMatch[1]
			}

			assert.Contains(t, fnStr, "ValidatePath",
				"function %s in %s performs file I/O but does not call ValidatePath",
				funcName, filepath.Base(path))
		}
	}
}
