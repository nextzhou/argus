package invariant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvariantZeroValue(t *testing.T) {
	var inv Invariant
	assert.Empty(t, inv.Version)
	assert.Empty(t, inv.ID)
	assert.Empty(t, inv.Description)
	assert.Empty(t, inv.Auto)
	assert.Nil(t, inv.Check)
	assert.Empty(t, inv.Prompt)
	assert.Empty(t, inv.Workflow)
}

func TestCheckStepZeroValue(t *testing.T) {
	var step CheckStep
	assert.Empty(t, step.Shell)
	assert.Empty(t, step.Description)
}
