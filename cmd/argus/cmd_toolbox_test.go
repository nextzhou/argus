package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolboxSubcommands(t *testing.T) {
	cmd := newToolboxCmd()

	// Verify parent command metadata
	assert.Equal(t, "toolbox", cmd.Use)
	assert.True(t, cmd.Hidden)

	// Verify exactly 4 subcommands
	subcommands := cmd.Commands()
	require.Len(t, subcommands, 4)

	// Expected subcommand names and their short descriptions
	expected := map[string]string{
		"jq":              "Run a jq-compatible query against JSON input from stdin",
		"yq":              "Run a yq-compatible query against YAML input from stdin",
		"touch-timestamp": "Write the current compact UTC timestamp to a file",
		"sha256sum":       "Compute SHA256 hash in coreutils format",
	}

	// Verify each subcommand exists with correct metadata
	found := make(map[string]bool)
	for _, sub := range subcommands {
		found[sub.Name()] = true
		expectedShort, ok := expected[sub.Name()]
		require.True(t, ok, "unexpected subcommand: %s", sub.Name())
		assert.Equal(t, expectedShort, sub.Short)
		assert.NotEmpty(t, sub.Short, "subcommand %s has empty Short description", sub.Name())
	}

	// Verify all expected subcommands were found
	for name := range expected {
		assert.True(t, found[name], "missing subcommand: %s", name)
	}
}
