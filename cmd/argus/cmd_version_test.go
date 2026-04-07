// Package main contains the CLI entry point and command definitions.
package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCommand(t *testing.T) {
	cmd := newVersionCmd("1.2.3")
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	err := cmd.Execute()
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "1.2.3")
	assert.Contains(t, output, "argus version")
}

func TestVersionCommandWithDevVersion(t *testing.T) {
	cmd := newVersionCmd("dev")
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	err := cmd.Execute()
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "dev")
	assert.Contains(t, output, "argus version")
}
