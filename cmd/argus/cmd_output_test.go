package main

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func withJSONFlag(args ...string) []string {
	if slices.Contains(args, "--json") {
		return append([]string(nil), args...)
	}

	withFlag := append([]string(nil), args...)
	withFlag = append(withFlag, "--json")
	return withFlag
}

func executeJSONCommand(t *testing.T, cmd *cobra.Command, args ...string) ([]byte, error) {
	t.Helper()

	var out bytes.Buffer
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(withJSONFlag(args...))

	err := cmd.Execute()
	return out.Bytes(), err
}

func executeJSONCommandWithInput(t *testing.T, cmd *cobra.Command, input io.Reader, args ...string) ([]byte, error) {
	t.Helper()

	var out bytes.Buffer
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	if input != nil {
		cmd.SetIn(input)
	}
	cmd.SetArgs(withJSONFlag(args...))

	err := cmd.Execute()
	return out.Bytes(), err
}

func executeTextCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, string, error) {
	t.Helper()

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func mustJSONInput(t *testing.T, payload map[string]string) string {
	t.Helper()

	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(data)
}

func mustJSONArray(t *testing.T, value any) []any {
	t.Helper()

	parsed, ok := value.([]any)
	require.True(t, ok, "value should be an array")
	return parsed
}

func mustJSONObject(t *testing.T, value any) map[string]any {
	t.Helper()

	parsed, ok := value.(map[string]any)
	require.True(t, ok, "value should be an object")
	return parsed
}

func mustJSONString(t *testing.T, value any) string {
	t.Helper()

	parsed, ok := value.(string)
	require.True(t, ok, "value should be a string")
	return parsed
}

func mustJSONBool(t *testing.T, value any) bool {
	t.Helper()

	parsed, ok := value.(bool)
	require.True(t, ok, "value should be a bool")
	return parsed
}
