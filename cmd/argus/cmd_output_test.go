package main

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func withJSONFlag(args ...string) []string {
	for _, arg := range args {
		if arg == "--json" {
			return append([]string(nil), args...)
		}
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

func executeTextCommandWithInput(t *testing.T, cmd *cobra.Command, input *bytes.Buffer, args ...string) (string, string, error) {
	t.Helper()

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if input != nil {
		cmd.SetIn(input)
	}
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func requireJSONOutput(t *testing.T, output []byte) map[string]any {
	t.Helper()

	var data map[string]any
	require.NoError(t, json.Unmarshal(output, &data), "output should be valid JSON: %s", string(output))
	return data
}
