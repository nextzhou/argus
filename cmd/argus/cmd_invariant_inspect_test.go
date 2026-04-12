package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeInvariantInspectCmd runs the invariant inspect command and captures stdout output.
func executeInvariantInspectCmd(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	return executeJSONCommand(t, newInvariantInspectCmd(), args...)
}

const validInvariantForInspect = `version: v0.1.0
id: my-check
order: 10
description: A valid invariant
auto: always
check:
  - shell: "true"
    description: "always passes"
prompt: "Fix it"
`

const validInvariantWithWorkflow = `version: v0.1.0
id: check-with-workflow
order: 20
description: Invariant referencing a workflow
auto: always
check:
  - shell: "true"
    description: "always passes"
workflow: my-workflow
prompt: "Run the workflow"
`

const validInvariantWithMissingWorkflow = `version: v0.1.0
id: check-missing-workflow
order: 30
description: Invariant referencing a non-existent workflow
auto: always
check:
  - shell: "true"
    description: "always passes"
workflow: nonexistent-workflow
prompt: "Run the workflow"
`

const builtinInvariantForInspect = `version: v0.1.0
id: argus-project-init
order: 40
description: Built-in invariant
auto: always
check:
  - shell: "true"
workflow: argus-project-init
`

const invalidInvariantYAMLForInspect = `not: valid: yaml: [broken`

const validWorkflowForInspect = `version: v0.1.0
id: my-workflow
description: Test workflow
jobs:
  - id: build
    prompt: "Build the project"
`

func TestInvariantInspect(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		setup      func(t *testing.T)
		wantErr    bool
		wantStatus string
		checkJSON  func(t *testing.T, data map[string]any)
	}{
		{
			name: "valid invariants produce ok report",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "my-check", validInvariantForInspect)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, true, data["valid"])

				fr := inspectEntryByBaseName(t, data, "my-check.yaml")
				assert.Equal(t, true, fr["valid"])
				assert.Equal(t, "my-check", fr["id"])
			},
		},
		{
			name: "invalid invariant YAML produces validation errors",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "bad-yaml", invalidInvariantYAMLForInspect)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, false, data["valid"])

				fr := inspectEntryByBaseName(t, data, "bad-yaml.yaml")
				assert.Equal(t, false, fr["valid"])

				findings := inspectFindings(t, fr)
				assert.NotEmpty(t, findings)
			},
		},
		{
			name: "mixed valid and invalid invariants",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "good", `version: v0.1.0
id: good
order: 10
description: A valid invariant
auto: always
check:
  - shell: "true"
    description: "always passes"
prompt: "Fix it"
`)
				writeInvariantFixture(t, "bad", invalidInvariantYAMLForInspect)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, false, data["valid"])

				goodFr := inspectEntryByBaseName(t, data, "good.yaml")
				assert.Equal(t, true, goodFr["valid"])

				badFr := inspectEntryByBaseName(t, data, "bad.yaml")
				assert.Equal(t, false, badFr["valid"])
				findings := inspectFindings(t, badFr)
				assert.NotEmpty(t, findings)
			},
		},
		{
			name:       "empty directory produces valid report",
			wantStatus: "ok",
			setup: func(t *testing.T) {
				require.NoError(t, os.MkdirAll(filepath.Join(".argus", "invariants"), 0o700))
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, true, data["valid"])

				assert.Empty(t, inspectEntries(t, data))
			},
		},
		{
			name:       "directory not found returns error",
			args:       []string{"/nonexistent/path/to/invariants"},
			wantErr:    true,
			wantStatus: "error",
			checkJSON: func(t *testing.T, data map[string]any) {
				msg, ok := data["message"].(string)
				require.True(t, ok)
				assert.Contains(t, msg, "reading invariant directory")
			},
		},
		{
			name: "invariant referencing existing workflow passes",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "check-with-workflow", validInvariantWithWorkflow)
				writeWorkflowFixture(t, "my-workflow", validWorkflowForInspect)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, true, data["valid"])

				fr := inspectEntryByBaseName(t, data, "check-with-workflow.yaml")
				assert.Equal(t, true, fr["valid"])
			},
		},
		{
			name: "invariant referencing non-existent workflow reports issue",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "check-missing-workflow", validInvariantWithMissingWorkflow)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, false, data["valid"])

				fr := inspectEntryByBaseName(t, data, "check-missing-workflow.yaml")
				assert.Equal(t, false, fr["valid"])

				findings := inspectFindings(t, fr)
				require.NotEmpty(t, findings)
				foundWorkflowError := false
				for _, finding := range findings {
					msg := finding["message"].(string)
					if msg == "referenced workflow \"nonexistent-workflow\" not found" {
						foundWorkflowError = true
					}
				}
				assert.True(t, foundWorkflowError, "should report missing workflow error")
			},
		},
		{
			name: "custom dir argument",
			args: []string{"custom-invariants"},
			setup: func(t *testing.T) {
				require.NoError(t, os.MkdirAll("custom-invariants", 0o700))
				require.NoError(t, os.WriteFile(
					filepath.Join("custom-invariants", "my-check.yaml"),
					[]byte(validInvariantForInspect), 0o600,
				))
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, true, data["valid"])

				assert.NotNil(t, inspectEntryByBaseName(t, data, "my-check.yaml"))
			},
		},
		{
			name: "built-in reserved invariant id is accepted",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "argus-project-init", builtinInvariantForInspect)
				writeWorkflowFixture(t, "argus-project-init", `version: v0.1.0
id: argus-project-init
description: Test workflow
jobs:
  - id: build
    prompt: "Build the project"
`)
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, true, data["valid"])

				fr := inspectEntryByBaseName(t, data, "argus-project-init.yaml")
				assert.Equal(t, true, fr["valid"])
				assert.Equal(t, "argus-project-init", fr["id"])
			},
		},
		{
			name: "invariant filename must match id",
			setup: func(t *testing.T) {
				require.NoError(t, os.MkdirAll(filepath.Join(".argus", "invariants"), 0o700))
				require.NoError(t, os.WriteFile(
					filepath.Join(".argus", "invariants", "wrong-name.yaml"),
					[]byte(validInvariantForInspect), 0o600,
				))
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, false, data["valid"])

				fr := inspectEntryByBaseName(t, data, "wrong-name.yaml")
				assert.Equal(t, false, fr["valid"])

				findings := inspectFindings(t, fr)
				require.NotEmpty(t, findings)
				found := false
				for _, finding := range findings {
					msg := finding["message"].(string)
					if msg == `invariant file name "wrong-name.yaml" must match invariant ID "my-check" (expected "my-check.yaml")` {
						found = true
					}
				}
				assert.True(t, found, "should report filename mismatch")
			},
		},
		{
			name: "misnamed workflow target is treated as missing",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "check-with-workflow", validInvariantWithWorkflow)
				require.NoError(t, os.MkdirAll(filepath.Join(".argus", "workflows"), 0o700))
				require.NoError(t, os.WriteFile(
					filepath.Join(".argus", "workflows", "wrong-name.yaml"),
					[]byte(validWorkflowForInspect), 0o600,
				))
			},
			wantStatus: "ok",
			checkJSON: func(t *testing.T, data map[string]any) {
				assert.Equal(t, false, data["valid"])

				fr := inspectEntryByBaseName(t, data, "check-with-workflow.yaml")
				assert.Equal(t, false, fr["valid"])

				findings := inspectFindings(t, fr)
				require.NotEmpty(t, findings)
				found := false
				for _, finding := range findings {
					msg := finding["message"].(string)
					if msg == `referenced workflow "my-workflow" not found` {
						found = true
					}
				}
				assert.True(t, found, "should report missing workflow error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if tt.setup != nil {
				tt.setup(t)
			}

			output, cmdErr := executeInvariantInspectCmd(t, tt.args...)

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

func TestInvariantInspectDefaultText(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T)
		wantContains   []string
		wantNoContains []string
	}{
		{
			name: "all valid shows success message",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "my-check", validInvariantForInspect)
			},
			wantContains: []string{
				"# Invariant Inspect",
				"All invariants valid.",
			},
		},
		{
			name: "validation errors show error details with filenames",
			setup: func(t *testing.T) {
				writeInvariantFixture(t, "bad-yaml", invalidInvariantYAMLForInspect)
			},
			wantContains: []string{
				"# Invariant Inspect",
				"Validation errors found:",
				"bad-yaml.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if tt.setup != nil {
				tt.setup(t)
			}

			stdout, stderr, err := executeTextCommand(t, newInvariantInspectCmd())
			require.NoError(t, err)
			assert.Empty(t, stderr)

			for _, want := range tt.wantContains {
				assert.Contains(t, stdout, want)
			}
			for _, notWant := range tt.wantNoContains {
				assert.NotContains(t, stdout, notWant)
			}
		})
	}
}

func TestBuildWorkflowChecker(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		checkIDs map[string]bool
	}{
		{
			name: "valid workflows dir with workflows",
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "workflows")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "my-workflow.yaml"),
					[]byte(validWorkflowForInspect), 0o600,
				))
				return dir
			},
			checkIDs: map[string]bool{
				"my-workflow": true,
				"unknown-wf":  false,
			},
		},
		{
			name: "nonexistent workflows dir returns false for all",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			checkIDs: map[string]bool{
				"anything":    false,
				"my-workflow": false,
			},
		},
		{
			name: "corrupt YAML is skipped but expected path still works",
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "workflows")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "my-workflow.yaml"),
					[]byte(validWorkflowForInspect), 0o600,
				))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "corrupt.yaml"),
					[]byte("not: valid: yaml: [broken"), 0o600,
				))
				return dir
			},
			checkIDs: map[string]bool{
				"my-workflow": true,
				"corrupt":     false,
			},
		},
		{
			name: "misnamed workflow file does not count as existing target",
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "workflows")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "wrong-name.yaml"),
					[]byte(validWorkflowForInspect), 0o600,
				))
				return dir
			},
			checkIDs: map[string]bool{
				"my-workflow": false,
			},
		},
		{
			name: "shared yaml is skipped",
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "workflows")
				require.NoError(t, os.MkdirAll(dir, 0o700))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "_shared.yaml"),
					[]byte(validWorkflowForInspect), 0o600,
				))
				return dir
			},
			checkIDs: map[string]bool{
				"my-workflow": false,
				"_shared":     false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			checker := buildWorkflowChecker("", dir)

			for id, want := range tt.checkIDs {
				assert.Equal(t, want, checker(id), "checker(%q) should return %v", id, want)
			}
		})
	}
}
