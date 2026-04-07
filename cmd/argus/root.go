package main

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates and returns the root command with all subcommands registered.
func NewRootCmd(version string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "argus",
		Short: "AI Agent workflow orchestration tool",
		Long:  "Argus is an AI Agent workflow orchestration tool that integrates with multiple AI Agents via their hook systems.",
	}

	// External commands (visible by default)
	rootCmd.AddCommand(newVersionCmd(version))
	rootCmd.AddCommand(newInstallCmd())
	rootCmd.AddCommand(newUninstallCmd())
	rootCmd.AddCommand(newDoctorCmd())

	// Internal commands (hidden by default)
	rootCmd.AddCommand(newTickCmd())
	rootCmd.AddCommand(newTrapCmd())
	rootCmd.AddCommand(newJobDoneCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newWorkflowCmd())
	rootCmd.AddCommand(newInvariantCmd())
	rootCmd.AddCommand(newToolboxCmd())

	// Add --all flag to show hidden commands
	var showAll bool
	rootCmd.PersistentFlags().BoolVar(&showAll, "all", false, "Show all commands including internal commands")

	// Override help function to handle --all flag
	originalHelpFunc := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if showAll {
			// Temporarily unhide all commands
			for _, c := range cmd.Commands() {
				c.Hidden = false
			}
		}
		originalHelpFunc(cmd, args)
	})

	return rootCmd
}

// newInstallCmd creates the install command (stub).
func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install Argus",
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
}

// newUninstallCmd creates the uninstall command (stub).
func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall Argus",
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
}

// newDoctorCmd creates the doctor command (stub).
func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostic checks",
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
}

// newTickCmd creates the tick command (internal, hidden).
func newTickCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "tick",
		Short:  "Tick the workflow state machine",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
	return cmd
}

// newTrapCmd creates the trap command (internal, hidden).
func newTrapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "trap",
		Short:  "Trap a signal or event",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
	return cmd
}

// newStatusCmd creates the status command (internal, hidden).
func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "status",
		Short:  "Show workflow status",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
	return cmd
}

// newWorkflowCmd creates the workflow command with subcommands.
func newWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "workflow",
		Short:  "Manage workflows",
		Hidden: true,
	}
	cmd.AddCommand(newWorkflowInspectCmd())
	cmd.AddCommand(newWorkflowStartCmd())
	cmd.AddCommand(newWorkflowListCmd())
	cmd.AddCommand(newWorkflowCancelCmd())
	return cmd
}

func newInvariantCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "invariant",
		Short:  "Manage invariants",
		Hidden: true,
	}
	cmd.AddCommand(newInvariantInspectCmd())
	return cmd
}
