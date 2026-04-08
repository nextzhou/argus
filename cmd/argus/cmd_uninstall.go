package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/install"
	"github.com/spf13/cobra"
)

// SEQUENCE-TEST: cmd_install_lifecycle_test.go
func newUninstallCmd() *cobra.Command {
	var yesFlag bool

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall Argus from the current project",
		RunE: func(_ *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			// Check if .argus/ exists
			argusDir := filepath.Join(cwd, ".argus")
			if _, err := os.Stat(argusDir); os.IsNotExist(err) {
				writeEnvelope(core.ErrorEnvelope("no Argus installation found in current directory"))
				return fmt.Errorf("no Argus installation found")
			}

			// Confirmation logic
			if !yesFlag {
				if !stdinIsTTY() {
					writeEnvelope(core.ErrorEnvelope("non-interactive mode requires --yes flag; use --yes to skip confirmation"))
					return fmt.Errorf("non-interactive mode requires --yes flag")
				}

				// Interactive: prompt user
				_, _ = os.Stderr.WriteString("This will remove .argus/ and Argus-managed skills. Continue? [y/N] ")
				var response string
				_, _ = fmt.Fscanln(os.Stdin, &response)
				if !strings.HasPrefix(strings.ToLower(response), "y") {
					writeEnvelope(core.ErrorEnvelope("uninstall cancelled"))
					return fmt.Errorf("uninstall cancelled")
				}
			}

			// 1. Remove .argus/ directory
			if err := os.RemoveAll(argusDir); err != nil {
				return fmt.Errorf("removing .argus: %w", err)
			}

			for _, skillPath := range install.SkillPaths() {
				skillsDir := filepath.Join(cwd, skillPath)
				if entries, err := os.ReadDir(skillsDir); err == nil {
					for _, entry := range entries {
						if entry.IsDir() && core.IsArgusReserved(entry.Name()) {
							_ = os.RemoveAll(filepath.Join(skillsDir, entry.Name()))
						}
					}
				}
			}

			// 3. Uninstall Agent hook configurations
			agents := []string{"claude-code", "codex", "opencode"}
			if err := install.UninstallHooks(cwd, agents); err != nil {
				return fmt.Errorf("uninstalling hooks: %w", err)
			}

			okBytes, err := core.OKEnvelope(map[string]string{
				"message": "Argus uninstalled successfully",
			})
			if err != nil {
				return fmt.Errorf("marshaling uninstall output: %w", err)
			}

			_, _ = os.Stdout.Write(okBytes)
			_, _ = os.Stdout.WriteString("\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompts")
	return cmd
}
