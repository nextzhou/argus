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
// SEQUENCE-TEST: cmd_workspace_lifecycle_test.go
func newUninstallCmd() *cobra.Command {
	var (
		yesFlag       bool
		workspaceFlag string
	)

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall Argus from the current project or remove a workspace registration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("workspace") {
				return runWorkspaceUninstall(workspaceFlag)
			}
			return runProjectUninstall(yesFlag)
		},
	}

	cmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompts")
	cmd.Flags().StringVar(&workspaceFlag, "workspace", "", "Workspace path to unregister")
	return cmd
}

func runWorkspaceUninstall(workspacePath string) error {
	if err := install.UninstallWorkspace(workspacePath); err != nil {
		writeEnvelope(core.ErrorEnvelope(err.Error()))
		return err
	}

	writeEnvelope(core.OKEnvelope(map[string]string{
		"message": "workspace unregistered successfully",
	}))
	return nil
}

func runProjectUninstall(yesFlag bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	argusDir := filepath.Join(cwd, ".argus")
	if _, err := os.Stat(argusDir); os.IsNotExist(err) {
		writeEnvelope(core.ErrorEnvelope("no Argus installation found in current directory"))
		return fmt.Errorf("no Argus installation found")
	}

	if !yesFlag {
		if !stdinIsTTY() {
			writeEnvelope(core.ErrorEnvelope("non-interactive mode requires --yes flag; use --yes to skip confirmation"))
			return fmt.Errorf("non-interactive mode requires --yes flag")
		}

		_, _ = os.Stderr.WriteString("This will remove .argus/ and Argus-managed skills. Continue? [y/N] ")
		var response string
		_, _ = fmt.Fscanln(os.Stdin, &response)
		if !strings.HasPrefix(strings.ToLower(response), "y") {
			writeEnvelope(core.ErrorEnvelope("uninstall cancelled"))
			return fmt.Errorf("uninstall cancelled")
		}
	}

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

	agents := []string{"claude-code", "codex", "opencode"}
	if err := install.UninstallHooks(cwd, agents); err != nil {
		return fmt.Errorf("uninstalling hooks: %w", err)
	}

	writeEnvelope(core.OKEnvelope(map[string]string{
		"message": "Argus uninstalled successfully",
	}))
	return nil
}
