package main

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates and returns the root command with all subcommands registered.
func NewRootCmd(version string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "argus",
		Short:         "AI Agent workflow orchestration tool",
		Long:          "Argus is an AI Agent workflow orchestration tool that integrates with multiple AI Agents via their hook systems.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// External commands (visible by default)
	rootCmd.AddCommand(newVersionCmd(version))
	rootCmd.AddCommand(newSetupCmd())
	rootCmd.AddCommand(newTeardownCmd())
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
	cmd.AddCommand(newWorkflowSnoozeCmd())
	return cmd
}

func newInvariantCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "invariant",
		Short:  "Manage invariants",
		Hidden: true,
	}
	cmd.AddCommand(newInvariantInspectCmd())
	cmd.AddCommand(newInvariantCheckCmd())
	cmd.AddCommand(newInvariantListCmd())
	return cmd
}
