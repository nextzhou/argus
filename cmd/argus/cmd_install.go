package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/install"
	workspacecfg "github.com/nextzhou/argus/internal/workspace"
	"github.com/spf13/cobra"
)

// checkStdinIsTTY wraps stdinIsTTY so tests can override the TTY guard.
var checkStdinIsTTY = stdinIsTTY

// SEQUENCE-TEST: cmd_install_lifecycle_test.go
// SEQUENCE-TEST: cmd_workspace_lifecycle_test.go
func newInstallCmd() *cobra.Command {
	var yesFlag bool
	var workspacePath string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Argus in the current project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("workspace") {
				if err := install.InstallWorkspace(workspacePath); err != nil {
					writeEnvelope(core.ErrorEnvelope(err.Error()))
					return err
				}

				normalizedPath, err := workspacecfg.NormalizePath(workspacePath)
				if err != nil {
					writeEnvelope(core.ErrorEnvelope(err.Error()))
					return err
				}

				okBytes, err := core.OKEnvelope(map[string]string{
					"message": "workspace registered",
					"path":    normalizedPath,
				})
				if err != nil {
					return fmt.Errorf("marshaling workspace install output: %w", err)
				}

				_, _ = os.Stdout.Write(okBytes)
				_, _ = os.Stdout.WriteString("\n")
				return nil
			}

			projectRoot, isSubdir, err := install.CheckInstallPreconditions()
			if err != nil {
				writeEnvelope(core.ErrorEnvelope(err.Error()))
				return err
			}

			if isSubdir && !yesFlag {
				confirmed, confirmErr := confirmSubdirectoryInstall(cmd, projectRoot, os.Stdin)
				if confirmErr != nil {
					writeEnvelope(core.ErrorEnvelope(confirmErr.Error()))
					return confirmErr
				}
				if !confirmed {
					cancelErr := fmt.Errorf("installation cancelled")
					writeEnvelope(core.ErrorEnvelope(cancelErr.Error()))
					return cancelErr
				}
			}

			if err := install.Install(projectRoot, yesFlag); err != nil {
				writeEnvelope(core.ErrorEnvelope(err.Error()))
				return err
			}

			okBytes, err := core.OKEnvelope(map[string]string{
				"message": "Argus installed successfully",
				"root":    projectRoot,
			})
			if err != nil {
				return fmt.Errorf("marshaling install output: %w", err)
			}

			_, _ = os.Stdout.Write(okBytes)
			_, _ = os.Stdout.WriteString("\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompts")
	cmd.Flags().StringVar(&workspacePath, "workspace", "", "Register a workspace path and install global hooks and skills")
	return cmd
}

func confirmSubdirectoryInstall(cmd *cobra.Command, projectRoot string, stdinReader io.Reader) (bool, error) {
	if !checkStdinIsTTY() {
		return false, fmt.Errorf("current directory is not the Git root — use --yes to install here anyway")
	}

	w := cmd.OutOrStdout()
	_, err := w.Write([]byte("Current directory is not the Git root.\n"))
	if err != nil {
		return false, fmt.Errorf("writing confirmation prompt: %w", err)
	}
	_, err = w.Write([]byte("Install Argus in this subdirectory instead: " + projectRoot + "\n"))
	if err != nil {
		return false, fmt.Errorf("writing confirmation prompt: %w", err)
	}
	_, err = w.Write([]byte("Continue? [y/N] "))
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

func writeEnvelope(envelope []byte, err error) {
	if err != nil {
		return
	}
	_, _ = os.Stdout.Write(envelope)
	_, _ = os.Stdout.WriteString("\n")
}
