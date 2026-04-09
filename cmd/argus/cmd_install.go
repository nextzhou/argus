package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nextzhou/argus/internal/install"
	"github.com/spf13/cobra"
)

// SEQUENCE-TEST: cmd_install_lifecycle_test.go
// SEQUENCE-TEST: cmd_workspace_lifecycle_test.go
func newInstallCmd() *cobra.Command {
	var yesFlag bool
	var jsonFlag bool
	var workspacePath string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Argus in the current project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("workspace") {
				preview, err := install.PrepareWorkspaceInstall(workspacePath)
				if err != nil {
					writeCommandError(cmd, jsonFlag, err.Error())
					return err
				}

				if !yesFlag {
					confirmed, confirmErr := confirmWorkspaceInstall(cmd, preview.Path, preview.AlreadyRegistered, os.Stdin, stdinIsTTY())
					if confirmErr != nil {
						writeCommandError(cmd, jsonFlag, confirmErr.Error())
						return confirmErr
					}
					if !confirmed {
						cancelErr := fmt.Errorf("workspace installation cancelled")
						if preview.AlreadyRegistered {
							cancelErr = fmt.Errorf("workspace refresh cancelled")
						}
						writeCommandError(cmd, jsonFlag, cancelErr.Error())
						return cancelErr
					}
				}

				result, err := install.InstallWorkspaceWithReport(workspacePath)
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

			projectRoot, isSubdir, err := install.CheckInstallPreconditions()
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return err
			}

			if isSubdir && !yesFlag {
				confirmed, confirmErr := confirmSubdirectoryInstall(cmd, projectRoot, os.Stdin, stdinIsTTY())
				if confirmErr != nil {
					writeCommandError(cmd, jsonFlag, confirmErr.Error())
					return confirmErr
				}
				if !confirmed {
					cancelErr := fmt.Errorf("installation cancelled")
					writeCommandError(cmd, jsonFlag, cancelErr.Error())
					return cancelErr
				}
			}

			result, err := install.InstallWithReport(projectRoot)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return err
			}

			output := lifecycleOutput{
				Message: "Argus installed successfully",
				Root:    result.Root,
				Report:  result.Report,
			}

			if jsonFlag {
				return writeJSONOK(cmd, output)
			}

			renderLifecycleText(cmd.OutOrStdout(), output, []string{
				"Run argus workflow start argus-init to complete project initialization.",
			})
			return nil
		},
	}

	cmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompts")
	cmd.Flags().StringVar(&workspacePath, "workspace", "", "Register a workspace path and install global hooks and skills")
	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func confirmSubdirectoryInstall(cmd *cobra.Command, projectRoot string, stdinReader io.Reader, isTTY bool) (bool, error) {
	return confirmWithPrompt(cmd, []string{
		"Current directory is not the Git root.",
		"Install Argus in this subdirectory instead: " + projectRoot,
	}, stdinReader, isTTY, "current directory is not the Git root — use --yes to install here anyway")
}

func confirmWorkspaceInstall(cmd *cobra.Command, normalizedPath string, alreadyRegistered bool, stdinReader io.Reader, isTTY bool) (bool, error) {
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
		"Argus will install global hooks and global skills for this user account.",
		"This does not install Argus into any project yet.",
		"Projects inside this workspace may be guided to run project-level Argus install.",
	}, stdinReader, isTTY, "workspace installation requires confirmation in interactive mode; use --yes to skip confirmation")
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

func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func reportHasChanges(report install.Report) bool {
	return len(report.Changes.Created) > 0 || len(report.Changes.Updated) > 0 || len(report.Changes.Removed) > 0
}
