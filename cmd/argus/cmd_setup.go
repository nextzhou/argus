package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nextzhou/argus/internal/lifecycle"
	"github.com/spf13/cobra"
)

// SEQUENCE-TEST: cmd_setup_lifecycle_test.go
// SEQUENCE-TEST: cmd_workspace_lifecycle_test.go
func newSetupCmd() *cobra.Command {
	var yesFlag bool
	var jsonFlag bool
	var workspacePath string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Set up project-level Argus in the current directory",
		RunE: func(cmd *cobra.Command, _ []string) error {
			input := cmd.InOrStdin()
			if cmd.Flags().Changed("workspace") {
				preview, err := lifecycle.PrepareWorkspaceSetup(workspacePath)
				if err != nil {
					writeCommandError(cmd, jsonFlag, err.Error())
					return err
				}

				if !yesFlag {
					confirmed, confirmErr := confirmWorkspaceSetup(cmd, preview.Path, preview.AlreadyRegistered, input, inputIsTTY(input))
					if confirmErr != nil {
						writeCommandError(cmd, jsonFlag, confirmErr.Error())
						return confirmErr
					}
					if !confirmed {
						cancelErr := fmt.Errorf("workspace setup cancelled")
						if preview.AlreadyRegistered {
							cancelErr = fmt.Errorf("workspace refresh cancelled")
						}
						writeCommandError(cmd, jsonFlag, cancelErr.Error())
						return cancelErr
					}
				}

				result, err := lifecycle.SetupWorkspaceWithReport(workspacePath)
				if err != nil {
					writeCommandError(cmd, jsonFlag, err.Error())
					return err
				}

				message := "workspace registered"
				if result.AlreadyRegistered {
					if reportHasChanges(result.Report) {
						message = "workspace already registered; global resources refreshed"
					} else {
						message = "workspace already registered; global resources already up to date"
					}
				}

				output := lifecycleOutput{
					Message: message,
					Path:    result.Path,
					Report:  result.Report,
				}

				if jsonFlag {
					return writeJSONOK(cmd, output)
				}

				renderLifecycleText(cmd.OutOrStdout(), output, nil)
				return nil
			}

			projectRoot, isSubdir, err := lifecycle.CheckSetupPreconditions()
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return err
			}

			if isSubdir && !yesFlag {
				confirmed, confirmErr := confirmSubdirectorySetup(cmd, projectRoot, input, inputIsTTY(input))
				if confirmErr != nil {
					writeCommandError(cmd, jsonFlag, confirmErr.Error())
					return confirmErr
				}
				if !confirmed {
					cancelErr := fmt.Errorf("setup cancelled")
					writeCommandError(cmd, jsonFlag, cancelErr.Error())
					return cancelErr
				}
			}

			result, err := lifecycle.SetupWithReport(projectRoot)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return err
			}

			output := lifecycleOutput{
				Message: "Project-level Argus set up successfully",
				Root:    result.Root,
				Report:  result.Report,
			}

			if jsonFlag {
				return writeJSONOK(cmd, output)
			}

			renderLifecycleText(cmd.OutOrStdout(), output, []string{
				"Run argus workflow start argus-project-init to complete project initialization.",
			})
			return nil
		},
	}

	cmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompts")
	cmd.Flags().StringVar(&workspacePath, "workspace", "", "Register a workspace path and set up global hooks, skills, and global artifacts")
	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func confirmSubdirectorySetup(cmd *cobra.Command, projectRoot string, stdinReader io.Reader, isTTY bool) (bool, error) {
	return confirmWithPrompt(cmd, []string{
		"Current directory is not the Git root.",
		"Set up project-level Argus in this subdirectory instead: " + projectRoot,
	}, stdinReader, isTTY, "current directory is not the Git root — use --yes to set up here anyway")
}

func confirmWorkspaceSetup(cmd *cobra.Command, normalizedPath string, alreadyRegistered bool, stdinReader io.Reader, isTTY bool) (bool, error) {
	if alreadyRegistered {
		return confirmWithPrompt(cmd, []string{
			"This workspace path is already registered:",
			"  " + normalizedPath,
			"",
			"Argus will refresh global hooks, global skills, and global artifacts for this user account.",
			"Use this after upgrading Argus or when built-in global resources need to be restored.",
		}, stdinReader, isTTY, "workspace refresh requires confirmation in interactive mode; use --yes to skip confirmation")
	}

	return confirmWithPrompt(cmd, []string{
		"This will register the workspace path:",
		"  " + normalizedPath,
		"",
		"Argus will set up global hooks, global skills, and global artifacts for this user account.",
		"This does not set up project-level Argus in any repository yet.",
		"Repositories inside this workspace may be guided to run project-level Argus setup.",
	}, stdinReader, isTTY, "workspace setup requires confirmation in interactive mode; use --yes to skip confirmation")
}

func confirmWithPrompt(cmd *cobra.Command, lines []string, stdinReader io.Reader, isTTY bool, nonTTYMessage string) (bool, error) {
	if !isTTY {
		return false, fmt.Errorf("%s", nonTTYMessage)
	}

	w := cmd.OutOrStdout()
	for _, line := range lines {
		_, err := w.Write([]byte(line + "\n"))
		if err != nil {
			return false, fmt.Errorf("writing confirmation prompt: %w", err)
		}
	}

	_, err := w.Write([]byte("Continue? [y/N] "))
	if err != nil {
		return false, fmt.Errorf("writing confirmation prompt: %w", err)
	}

	reader := bufio.NewReader(stdinReader)
	line, err := reader.ReadString('\n')
	if err != nil && err.Error() != "EOF" {
		return false, fmt.Errorf("reading confirmation input: %w", err)
	}

	response := strings.TrimSpace(line)
	return strings.EqualFold(response, "y") || strings.EqualFold(response, "yes"), nil
}

func inputIsTTY(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func reportHasChanges(report lifecycle.Report) bool {
	return len(report.Changes.Created) > 0 || len(report.Changes.Updated) > 0 || len(report.Changes.Removed) > 0
}
