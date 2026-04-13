package main

import (
	"fmt"
	"io"

	"github.com/nextzhou/argus/internal/core"
	"github.com/spf13/cobra"
)

func bindJSONFlag(cmd *cobra.Command, jsonFlag *bool) {
	cmd.Flags().BoolVar(jsonFlag, "json", false, "Output structured JSON")
}

func requireYesForJSON(cmd *cobra.Command, jsonFlag bool, yesFlag bool, operation string) error {
	if !jsonFlag || yesFlag {
		return nil
	}

	msg := fmt.Sprintf("%s requires --yes when --json is used; --json is non-interactive", operation)
	writeCommandError(cmd, true, msg)

	return fmt.Errorf("%s", msg)
}

func writeJSONEnvelope(w io.Writer, envelope []byte) error {
	if _, err := w.Write(envelope); err != nil {
		return fmt.Errorf("writing JSON output: %w", err)
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return fmt.Errorf("writing JSON output newline: %w", err)
	}

	return nil
}

func writeJSONOK(cmd *cobra.Command, data any) error {
	okBytes, err := core.OKEnvelope(data)
	if err != nil {
		return fmt.Errorf("marshaling JSON output: %w", err)
	}

	return writeJSONEnvelope(cmd.OutOrStdout(), okBytes)
}

func writeJSONError(cmd *cobra.Command, msg string) error {
	errBytes, err := core.ErrorEnvelope(msg)
	if err != nil {
		return fmt.Errorf("marshaling JSON error output: %w", err)
	}

	return writeJSONEnvelope(cmd.OutOrStdout(), errBytes)
}

func writeJSONErrorDetails(cmd *cobra.Command, msg string, details any) error {
	errBytes, err := core.ErrorEnvelopeWithDetails(msg, details)
	if err != nil {
		return fmt.Errorf("marshaling JSON error output: %w", err)
	}

	return writeJSONEnvelope(cmd.OutOrStdout(), errBytes)
}

func writeCommandError(cmd *cobra.Command, jsonOutput bool, msg string) {
	if jsonOutput {
		_ = writeJSONError(cmd, msg)
		return
	}

	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), msg)
}

func writeCommandErrorDetails(cmd *cobra.Command, jsonOutput bool, msg string, details any) {
	if jsonOutput {
		_ = writeJSONErrorDetails(cmd, msg, details)
		return
	}

	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), msg)
}
