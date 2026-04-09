package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextzhou/argus/internal/install"
	"github.com/spf13/cobra"
)

// SEQUENCE-TEST: cmd_install_lifecycle_test.go
// SEQUENCE-TEST: cmd_workspace_lifecycle_test.go
func newUninstallCmd() *cobra.Command {
	var (
		yesFlag       bool
		jsonFlag      bool
		workspaceFlag string
	)

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall Argus from the current project or remove a workspace registration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("workspace") {
				return runWorkspaceUninstall(cmd, workspaceFlag, yesFlag, jsonFlag)
			}
			return runProjectUninstall(cmd, yesFlag, jsonFlag)
		},
	}

	cmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompts")
	cmd.Flags().StringVar(&workspaceFlag, "workspace", "", "Workspace path to unregister")
	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func runWorkspaceUninstall(cmd *cobra.Command, workspacePath string, yesFlag bool, jsonFlag bool) error {
	preview, err := install.PrepareWorkspaceUninstall(workspacePath)
	if err != nil {
		writeCommandError(cmd, jsonFlag, err.Error())
		return err
	}

	if !yesFlag {
		confirmed, confirmErr := confirmWorkspaceUninstall(cmd, preview.Path, preview.IsLast, os.Stdin, stdinIsTTY())
		if confirmErr != nil {
			writeCommandError(cmd, jsonFlag, confirmErr.Error())
			return confirmErr
		}
		if !confirmed {
			cancelErr := fmt.Errorf("workspace uninstallation cancelled")
			writeCommandError(cmd, jsonFlag, cancelErr.Error())
			return cancelErr
		}
	}

	result, err := install.UninstallWorkspaceWithReport(workspacePath)
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

func runProjectUninstall(cmd *cobra.Command, yesFlag bool, jsonFlag bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	argusDir := filepath.Join(cwd, ".argus")
	if _, err := os.Stat(argusDir); os.IsNotExist(err) {
		writeCommandError(cmd, jsonFlag, "no Argus installation found in current directory")
		return fmt.Errorf("no Argus installation found")
	}

	if !yesFlag {
		if !stdinIsTTY() {
			writeCommandError(cmd, jsonFlag, "non-interactive mode requires --yes flag; use --yes to skip confirmation")
			return fmt.Errorf("non-interactive mode requires --yes flag")
		}

		_, _ = cmd.ErrOrStderr().Write([]byte("This will remove .argus/ and Argus-managed skills. Continue? [y/N] "))
		var response string
		_, _ = fmt.Fscanln(os.Stdin, &response)
		if !strings.HasPrefix(strings.ToLower(response), "y") {
			writeCommandError(cmd, jsonFlag, "uninstall cancelled")
			return fmt.Errorf("uninstall cancelled")
		}
	}

	report, err := install.UninstallProject(cwd)
	if err != nil {
		writeCommandError(cmd, jsonFlag, err.Error())
		return err
	}

	output := lifecycleOutput{
		Message: "Argus uninstalled successfully",
		Report:  report,
	}

	if jsonFlag {
		return writeJSONOK(cmd, output)
	}

	renderLifecycleText(cmd.OutOrStdout(), output, nil)
	return nil
}

func confirmWorkspaceUninstall(cmd *cobra.Command, normalizedPath string, isLast bool, stdinReader io.Reader, isTTY bool) (bool, error) {
	lines := []string{
		"This will unregister the workspace path:",
		"  " + normalizedPath,
		"",
	}
	if isLast {
		lines = append(lines,
			"No registered workspaces will remain.",
			"Argus will remove global hooks and global skills for this user account.",
		)
	} else {
		lines = append(lines,
			"Argus will stop guiding projects inside this workspace via global hooks.",
		)
	}

	return confirmWithPrompt(cmd, lines, stdinReader, isTTY, "workspace uninstallation requires confirmation in interactive mode; use --yes to skip confirmation")
}
