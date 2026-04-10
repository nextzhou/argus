package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextzhou/argus/internal/lifecycle"
	"github.com/spf13/cobra"
)

// SEQUENCE-TEST: cmd_setup_lifecycle_test.go
// SEQUENCE-TEST: cmd_workspace_lifecycle_test.go
func newTeardownCmd() *cobra.Command {
	var (
		yesFlag       bool
		jsonFlag      bool
		workspaceFlag string
	)

	cmd := &cobra.Command{
		Use:   "teardown",
		Short: "Remove project-level Argus setup or unregister a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("workspace") {
				return runWorkspaceTeardown(cmd, workspaceFlag, yesFlag, jsonFlag)
			}
			return runProjectTeardown(cmd, yesFlag, jsonFlag)
		},
	}

	cmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompts")
	cmd.Flags().StringVar(&workspaceFlag, "workspace", "", "Workspace path to unregister")
	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func runWorkspaceTeardown(cmd *cobra.Command, workspacePath string, yesFlag bool, jsonFlag bool) error {
	preview, err := lifecycle.PrepareWorkspaceTeardown(workspacePath)
	if err != nil {
		writeCommandError(cmd, jsonFlag, err.Error())
		return err
	}

	if !yesFlag {
		input := cmd.InOrStdin()
		confirmed, confirmErr := confirmWorkspaceTeardown(cmd, preview.Path, preview.IsLast, input, inputIsTTY(input))
		if confirmErr != nil {
			writeCommandError(cmd, jsonFlag, confirmErr.Error())
			return confirmErr
		}
		if !confirmed {
			cancelErr := fmt.Errorf("workspace teardown cancelled")
			writeCommandError(cmd, jsonFlag, cancelErr.Error())
			return cancelErr
		}
	}

	result, err := lifecycle.TeardownWorkspaceWithReport(workspacePath)
	if err != nil {
		writeCommandError(cmd, jsonFlag, err.Error())
		return err
	}

	output := lifecycleOutput{
		Message: "workspace unregistered successfully",
		Path:    result.Path,
		Report:  result.Report,
	}

	if jsonFlag {
		return writeJSONOK(cmd, output)
	}

	renderLifecycleText(cmd.OutOrStdout(), output, nil)
	return nil
}

func runProjectTeardown(cmd *cobra.Command, yesFlag bool, jsonFlag bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	argusDir := filepath.Join(cwd, ".argus")
	if _, err := os.Stat(argusDir); os.IsNotExist(err) {
		writeCommandError(cmd, jsonFlag, "no project-level Argus setup found in current directory")
		return fmt.Errorf("no project-level Argus setup found")
	}

	if !yesFlag {
		input := cmd.InOrStdin()
		if !inputIsTTY(input) {
			writeCommandError(cmd, jsonFlag, "non-interactive mode requires --yes flag; use --yes to skip confirmation")
			return fmt.Errorf("non-interactive mode requires --yes flag")
		}

		_, _ = cmd.ErrOrStderr().Write([]byte("This will remove project-level Argus setup (.argus/ and Argus-managed skills). Continue? [y/N] "))
		var response string
		_, _ = fmt.Fscanln(input, &response)
		if !strings.HasPrefix(strings.ToLower(response), "y") {
			writeCommandError(cmd, jsonFlag, "teardown cancelled")
			return fmt.Errorf("teardown cancelled")
		}
	}

	report, err := lifecycle.TeardownProject(cwd)
	if err != nil {
		writeCommandError(cmd, jsonFlag, err.Error())
		return err
	}

	output := lifecycleOutput{
		Message: "Project-level Argus setup removed successfully",
		Report:  report,
	}

	if jsonFlag {
		return writeJSONOK(cmd, output)
	}

	renderLifecycleText(cmd.OutOrStdout(), output, nil)
	return nil
}

func confirmWorkspaceTeardown(cmd *cobra.Command, normalizedPath string, isLast bool, stdinReader io.Reader, isTTY bool) (bool, error) {
	lines := []string{
		"This will unregister the workspace path:",
		"  " + normalizedPath,
		"",
	}
	if isLast {
		lines = append(lines,
			"No registered workspaces will remain.",
			"Argus will remove global hooks, global skills, global bootstrap artifacts, and the managed ~/.config/argus/ root for this user account.",
		)
	} else {
		lines = append(lines,
			"Argus will stop guiding repositories inside this workspace via global hooks.",
		)
	}

	return confirmWithPrompt(cmd, lines, stdinReader, isTTY, "workspace teardown requires confirmation in interactive mode; use --yes to skip confirmation")
}
