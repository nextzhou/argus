package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultHelpHidesInternalCommands(t *testing.T) {
	cmd := NewRootCmd("test")
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"help"})
	err := cmd.Execute()
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "install")
	assert.Contains(t, output, "version")
	assert.NotContains(t, output, "tick")
	assert.NotContains(t, output, "job-done")
}

func TestHelpAllShowsInternalCommands(t *testing.T) {
	cmd := NewRootCmd("test")
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"help", "--all"})
	err := cmd.Execute()
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "tick")
	assert.Contains(t, output, "job-done")
	assert.Contains(t, output, "install")
}

func TestRootCommandShortDescription(t *testing.T) {
	cmd := NewRootCmd("test")
	assert.Equal(t, "argus", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
}
