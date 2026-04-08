package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/install"
	"github.com/spf13/cobra"
)

// SEQUENCE-TEST: cmd_install_lifecycle_test.go
func newInstallCmd() *cobra.Command {
	var yesFlag bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Argus in the current project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectRoot, isSubdir, err := install.CheckInstallPreconditions()
			if err != nil {
				writeEnvelope(core.ErrorEnvelope(err.Error()))
				return err
			}

			if isSubdir && !yesFlag {
				confirmed, confirmErr := confirmSubdirectoryInstall(cmd, projectRoot)
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
	return cmd
}

func confirmSubdirectoryInstall(cmd *cobra.Command, projectRoot string) (bool, error) {
	if !stdinIsTTY() {
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

	reader := bufio.NewReader(os.Stdin)
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
