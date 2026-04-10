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
				assert.True(t, data["valid"].(bool))
				files, ok := data["files"].(map[string]any)
				require.True(t, ok, "files should be an object")
				require.Len(t, files, 1)

				buildFile, ok := files["build.yaml"].(map[string]any)
				require.True(t, ok, "build.yaml should exist")
				assert.True(t, buildFile["valid"].(bool))

				workflow, ok := buildFile["workflow"].(map[string]any)
				require.True(t, ok, "workflow metadata should exist")
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
				assert.False(t, data["valid"].(bool))
				files, ok := data["files"].(map[string]any)
				require.True(t, ok, "files should be an object")

				badFile, ok := files["bad.yaml"].(map[string]any)
				require.True(t, ok, "bad.yaml should exist")
				assert.False(t, badFile["valid"].(bool))
				errors, ok := badFile["errors"].([]any)
				require.True(t, ok, "errors should be an array")
				require.NotEmpty(t, errors)
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
				assert.False(t, data["valid"].(bool))
				files, ok := data["files"].(map[string]any)
				require.True(t, ok, "files should be an object")

				validFile, ok := files["valid.yaml"].(map[string]any)
				require.True(t, ok, "valid.yaml should exist")
				assert.True(t, validFile["valid"].(bool))

				invalidFile, ok := files["invalid.yaml"].(map[string]any)
				require.True(t, ok, "invalid.yaml should exist")
				assert.False(t, invalidFile["valid"].(bool))
				errors, ok := invalidFile["errors"].([]any)
				require.True(t, ok, "errors should be an array")
				require.NotEmpty(t, errors)
			},
		},
		{
			name:       "empty directory returns valid report",
			wantStatus: "ok",
			setupFiles: nil,
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.True(t, data["valid"].(bool))
				files, ok := data["files"].(map[string]any)
				require.True(t, ok, "files should be an object")
				assert.Empty(t, files)
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
				assert.False(t, data["valid"].(bool))
				files, ok := data["files"].(map[string]any)
				require.True(t, ok, "files should be an object")

				firstFile, ok := files["first.yaml"].(map[string]any)
				require.True(t, ok, "first.yaml should exist")
				assert.False(t, firstFile["valid"].(bool))

				secondFile, ok := files["second.yaml"].(map[string]any)
				require.True(t, ok, "second.yaml should exist")
				assert.False(t, secondFile["valid"].(bool))
			},
		},
		{
			name:       "reserved argus- prefix rejected",
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
				assert.False(t, data["valid"].(bool))
				files, ok := data["files"].(map[string]any)
				require.True(t, ok, "files should be an object")

				reservedFile, ok := files["reserved.yaml"].(map[string]any)
				require.True(t, ok, "reserved.yaml should exist")
				assert.False(t, reservedFile["valid"].(bool))
				errors, ok := reservedFile["errors"].([]any)
				require.True(t, ok, "errors should be an array")
				require.NotEmpty(t, errors)
				assert.Contains(t, errors[0].(map[string]any)["message"], "reserved")
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
				assert.False(t, data["valid"].(bool))
				files, ok := data["files"].(map[string]any)
				require.True(t, ok, "files should be an object")

				badFile, ok := files["bad-template.yaml"].(map[string]any)
				require.True(t, ok, "bad-template.yaml should exist")
				assert.False(t, badFile["valid"].(bool))
				errors, ok := badFile["errors"].([]any)
				require.True(t, ok, "errors should be an array")
				require.NotEmpty(t, errors)
				assert.Contains(t, errors[0].(map[string]any)["message"], "template")
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
				assert.False(t, data["valid"].(bool))
				files, ok := data["files"].(map[string]any)
				require.True(t, ok, "files should be an object")

				buildFile, ok := files["build.yaml"].(map[string]any)
				require.True(t, ok, "build.yaml should exist")
				assert.False(t, buildFile["valid"].(bool))
				errors, ok := buildFile["errors"].([]any)
				require.True(t, ok, "errors should be an array")
				require.NotEmpty(t, errors)
				assert.Contains(t, errors[0].(map[string]any)["message"], "not found")
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
				assert.Contains(t, output, "# Workflow Inspect")
				assert.Contains(t, output, "Validation errors found")
				assert.Contains(t, output, "bad.yaml")
			},
		},
		{
			name:       "empty directory default text output",
			setupFiles: nil,
			checkOutput: func(t *testing.T, output string) {
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

	files, ok := data["files"].(map[string]any)
	require.True(t, ok, "files should be an object")
	require.Len(t, files, 1)
	assert.NotNil(t, files["build.yaml"])
	assert.Nil(t, files["README.md"])
	assert.Nil(t, files["notes.txt"])
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

	files, ok := data["files"].(map[string]any)
	require.True(t, ok, "files should be an object")
	require.Len(t, files, 1)
	assert.NotNil(t, files["build.yaml"])
	assert.Nil(t, files["nested.yaml"])
}
