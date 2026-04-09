package hook

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTemplate_Success(t *testing.T) {
	data := struct {
		WorkflowID string
		JobID      string
		Progress   string
	}{
		WorkflowID: "release",
		JobID:      "run_tests",
		Progress:   "2/5",
	}

	result, err := renderTemplate("prompts/tick-minimal.md.tmpl", data)

	require.NoError(t, err)
	assertHookSafeTickText(t, result)
	assert.Contains(t, result, "Argus:")
	assert.Contains(t, result, "release")
	assert.Contains(t, result, "run_tests")
	assert.Contains(t, result, "2/5")
	assert.Contains(t, result, "argus job-done")
}

func TestRenderTemplate_NotFound(t *testing.T) {
	_, err := renderTemplate("prompts/nonexistent.md.tmpl", nil)

	require.Error(t, err)
	assert.ErrorContains(t, err, "reading template")
}

func TestRenderTemplate_ExecutionError(t *testing.T) {
	// tick-minimal.md.tmpl expects .WorkflowID, .JobID, .Progress fields.
	// Passing a struct without those fields causes Execute to fail.
	data := struct{ Unrelated string }{Unrelated: "value"}

	_, err := renderTemplate("prompts/tick-minimal.md.tmpl", data)

	require.Error(t, err)
	assert.ErrorContains(t, err, "executing template")
}
