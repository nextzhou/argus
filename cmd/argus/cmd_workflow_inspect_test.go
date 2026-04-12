package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeWorkflowInspectCmd runs the workflow inspect command and captures stdout output.
func executeWorkflowInspectCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newWorkflowInspectCmd(), args...)
}

func TestWorkflowInspect(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles map[string]string
		args       []string
		wantErr    bool
		wantStatus string
		checkJSON  func(t *testing.T, data map[string]any)
	}{
		{
			name:       "valid workflow returns ok status",
			wantStatus: "ok",
			setupFiles: map[string]string{
				"build.yaml": `version: v0.1.0
id: build
description: Build workflow
jobs:
  - id: compile
    prompt: "Compile the code"
  - id: test
    prompt: "Run tests"
`,
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				assert.True(t, mustJSONBool(t, data["valid"]))
				entries := inspectEntries(t, data)
				require.Len(t, entries, 1)

				buildFile := inspectEntryByBaseName(t, data, "build.yaml")
				assert.True(t, mustJSONBool(t, buildFile["valid"]))

				workflow := mustJSONObject(t, buildFile["workflow"])
				assert.Equal(t, "build", workflow["id"])
				assert.InDelta(t, 2, workflow["jobs"], 0)
			},
		},
		{
			name:       "invalid yaml returns ok status with errors in report",
			wantErr:    false,
			wantStatus: "ok",
			setupFiles: map[string]string{
				"bad.yaml": `version: v0.1.0
id: bad
jobs:
  - id: step1
    prompt: "Step 1"
  - invalid yaml here [[[
`,
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				assert.False(t, mustJSONBool(t, data["valid"]))

				badFile := inspectEntryByBaseName(t, data, "bad.yaml")
				assert.False(t, mustJSONBool(t, badFile["valid"]))
				findings := inspectFindings(t, badFile)
				require.NotEmpty(t, findings)
			},
		},
		{
			name:       "mixed valid and invalid workflows",
			wantErr:    false,
			wantStatus: "ok",
			setupFiles: map[string]string{
				"valid.yaml": `version: v0.1.0
id: valid
description: Valid workflow
jobs:
  - id: step1
    prompt: "Step 1"
`,
				"invalid.yaml": `version: v0.1.0
id: invalid
jobs:
  - id: step1
    prompt: "Step 1"
  - bad yaml [[[
`,
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				assert.False(t, mustJSONBool(t, data["valid"]))

				validFile := inspectEntryByBaseName(t, data, "valid.yaml")
				assert.True(t, mustJSONBool(t, validFile["valid"]))

				invalidFile := inspectEntryByBaseName(t, data, "invalid.yaml")
				assert.False(t, mustJSONBool(t, invalidFile["valid"]))
				findings := inspectFindings(t, invalidFile)
				require.NotEmpty(t, findings)
			},
		},
		{
			name:       "empty directory returns valid report",
			wantStatus: "ok",
			setupFiles: nil,
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				assert.True(t, mustJSONBool(t, data["valid"]))
				assert.Empty(t, inspectEntries(t, data))
			},
		},
		{
			name:       "duplicate workflow ids detected",
			wantErr:    false,
			wantStatus: "ok",
			setupFiles: map[string]string{
				"first.yaml": `version: v0.1.0
id: duplicate
description: First workflow
jobs:
  - id: step1
    prompt: "Step 1"
`,
				"second.yaml": `version: v0.1.0
id: duplicate
description: Second workflow
jobs:
  - id: step1
    prompt: "Step 1"
`,
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				assert.False(t, mustJSONBool(t, data["valid"]))

				firstFile := inspectEntryByBaseName(t, data, "first.yaml")
				assert.False(t, mustJSONBool(t, firstFile["valid"]))

				secondFile := inspectEntryByBaseName(t, data, "second.yaml")
				assert.False(t, mustJSONBool(t, secondFile["valid"]))
			},
		},
		{
			name:       "unknown reserved argus- prefix rejected",
			wantErr:    false,
			wantStatus: "ok",
			setupFiles: map[string]string{
				"reserved.yaml": `version: v0.1.0
id: argus-reserved
description: Reserved workflow
jobs:
  - id: step1
    prompt: "Step 1"
`,
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				assert.False(t, mustJSONBool(t, data["valid"]))

				reservedFile := inspectEntryByBaseName(t, data, "reserved.yaml")
				assert.False(t, mustJSONBool(t, reservedFile["valid"]))
				findings := inspectFindings(t, reservedFile)
				require.NotEmpty(t, findings)
				assert.Contains(t, findings[0]["message"], "reserved")
			},
		},
		{
			name:       "built-in reserved workflow id is accepted",
			wantStatus: "ok",
			setupFiles: map[string]string{
				"argus-project-init.yaml": `version: v0.1.0
id: argus-project-init
description: Built-in workflow
jobs:
  - id: step1
    prompt: "Step 1"
`,
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				assert.True(t, mustJSONBool(t, data["valid"]))

				builtinFile := inspectEntryByBaseName(t, data, "argus-project-init.yaml")
				assert.True(t, mustJSONBool(t, builtinFile["valid"]))
			},
		},
		{
			name:       "workflow filename must match id",
			wantErr:    false,
			wantStatus: "ok",
			setupFiles: map[string]string{
				"wrong-name.yaml": `version: v0.1.0
id: release
description: Release workflow
jobs:
  - id: step1
    prompt: "Step 1"
`,
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				assert.False(t, mustJSONBool(t, data["valid"]))

				file := inspectEntryByBaseName(t, data, "wrong-name.yaml")
				assert.False(t, mustJSONBool(t, file["valid"]))
				findings := inspectFindings(t, file)
				require.NotEmpty(t, findings)
				assert.Contains(t, findings[0]["message"], `expected "release.yaml"`)
			},
		},
		{
			name:       "invalid template syntax detected",
			wantErr:    false,
			wantStatus: "ok",
			setupFiles: map[string]string{
				"bad-template.yaml": `version: v0.1.0
id: bad-template
description: Bad template workflow
jobs:
  - id: step1
    prompt: "Invalid template {{.Foo"
`,
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				assert.False(t, mustJSONBool(t, data["valid"]))

				badFile := inspectEntryByBaseName(t, data, "bad-template.yaml")
				assert.False(t, mustJSONBool(t, badFile["valid"]))
				findings := inspectFindings(t, badFile)
				require.NotEmpty(t, findings)
				assert.Contains(t, findings[0]["message"], "template")
			},
		},
		{
			name:       "missing shared ref detected",
			wantErr:    false,
			wantStatus: "ok",
			setupFiles: map[string]string{
				"_shared.yaml": `lint:
  id: lint
  prompt: "Run lint"
`,
				"build.yaml": `version: v0.1.0
id: build
description: Build workflow
jobs:
  - id: compile
    prompt: "Compile"
  - ref: nonexistent
`,
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				assert.False(t, mustJSONBool(t, data["valid"]))

				buildFile := inspectEntryByBaseName(t, data, "build.yaml")
				assert.False(t, mustJSONBool(t, buildFile["valid"]))
				findings := inspectFindings(t, buildFile)
				require.NotEmpty(t, findings)
				assert.Contains(t, findings[0]["message"], "not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if tt.setupFiles != nil {
				workflowsDir := filepath.Join(".argus", "workflows")
				require.NoError(t, os.MkdirAll(workflowsDir, 0o700))
				for name, content := range tt.setupFiles {
					fullPath := filepath.Join(workflowsDir, name)
					require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o700))
					require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o600))
				}
			} else {
				workflowsDir := filepath.Join(".argus", "workflows")
				require.NoError(t, os.MkdirAll(workflowsDir, 0o700))
			}

			output, cmdErr := executeWorkflowInspectCmd(t, tt.args...)

			if tt.wantErr {
				require.Error(t, cmdErr)
			} else {
				require.NoError(t, cmdErr)
			}

			var data map[string]any
			require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
			assert.Equal(t, tt.wantStatus, data["status"])

			if tt.checkJSON != nil {
				tt.checkJSON(t, data)
			}
		})
	}
}

func TestWorkflowInspectDefaultText(t *testing.T) {
	tests := []struct {
		name        string
		setupFiles  map[string]string
		checkOutput func(t *testing.T, output string)
	}{
		{
			name: "all valid workflows default text output",
			setupFiles: map[string]string{
				"build.yaml": `version: v0.1.0
id: build
description: Build workflow
jobs:
  - id: compile
    prompt: "Compile"
`,
			},
			checkOutput: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "# Workflow Inspect")
				assert.Contains(t, output, "All workflows valid")
			},
		},
		{
			name: "validation errors default text output",
			setupFiles: map[string]string{
				"bad.yaml": `version: v0.1.0
id: bad
jobs:
  - id: step1
    prompt: "Step 1"
  - bad yaml [[[
`,
			},
			checkOutput: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "# Workflow Inspect")
				assert.Contains(t, output, "Validation errors found")
				assert.Contains(t, output, "bad.yaml")
			},
		},
		{
			name:       "empty directory default text output",
			setupFiles: nil,
			checkOutput: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "# Workflow Inspect")
				assert.Contains(t, output, "All workflows valid")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if tt.setupFiles != nil {
				workflowsDir := filepath.Join(".argus", "workflows")
				require.NoError(t, os.MkdirAll(workflowsDir, 0o700))
				for name, content := range tt.setupFiles {
					fullPath := filepath.Join(workflowsDir, name)
					require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o700))
					require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o600))
				}
			} else {
				workflowsDir := filepath.Join(".argus", "workflows")
				require.NoError(t, os.MkdirAll(workflowsDir, 0o700))
			}

			outputStr, stderr, err := executeTextCommand(t, newWorkflowInspectCmd())
			require.NoError(t, err)
			assert.Empty(t, stderr)

			if tt.checkOutput != nil {
				tt.checkOutput(t, outputStr)
			}
		})
	}
}

func TestWorkflowInspectNonYAMLFilesIgnored(t *testing.T) {
	t.Chdir(t.TempDir())

	workflowsDir := filepath.Join(".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o700))

	require.NoError(t, os.WriteFile(
		filepath.Join(workflowsDir, "README.md"),
		[]byte("# Workflows"), 0o600,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(workflowsDir, "notes.txt"),
		[]byte("Some notes"), 0o600,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(workflowsDir, "build.yaml"),
		[]byte(`version: v0.1.0
id: build
description: Build workflow
jobs:
  - id: compile
    prompt: "Compile"
`), 0o600,
	))

	output, err := executeWorkflowInspectCmd(t)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data))
	assert.Equal(t, "ok", data["status"])

	entries := inspectEntries(t, data)
	require.Len(t, entries, 1)
	assert.NotNil(t, inspectEntryByBaseName(t, data, "build.yaml"))
}

func TestWorkflowInspectDirectoryNotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	output, cmdErr := executeWorkflowInspectCmd(t)

	require.Error(t, cmdErr)

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data))
	assert.Equal(t, "error", data["status"])
	assert.NotEmpty(t, data["message"])
}

func TestWorkflowInspectCustomDirNotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	output, cmdErr := executeWorkflowInspectCmd(t, "nonexistent-dir")

	require.Error(t, cmdErr)

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data))
	assert.Equal(t, "error", data["status"])
	assert.NotEmpty(t, data["message"])
}

func TestWorkflowInspectSubdirectoryIgnored(t *testing.T) {
	t.Chdir(t.TempDir())

	workflowsDir := filepath.Join(".argus", "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o700))

	subDir := filepath.Join(workflowsDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(subDir, "nested.yaml"),
		[]byte(`version: v0.1.0
id: nested
jobs:
  - id: step1
    prompt: "Step 1"
`), 0o600,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(workflowsDir, "build.yaml"),
		[]byte(`version: v0.1.0
id: build
jobs:
  - id: compile
    prompt: "Compile"
`), 0o600,
	))

	output, err := executeWorkflowInspectCmd(t)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data))
	assert.Equal(t, "ok", data["status"])

	entries := inspectEntries(t, data)
	require.Len(t, entries, 1)
	assert.NotNil(t, inspectEntryByBaseName(t, data, "build.yaml"))
}
