package main

import (
	"os"

	"github.com/nextzhou/argus/internal/toolbox"
	"github.com/spf13/cobra"
)

func newToolboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "toolbox",
		Short:  "Built-in tool utilities",
		Hidden: true,
	}

	cmd.AddCommand(newToolboxJQCmd())
	cmd.AddCommand(newToolboxYQCmd())
	cmd.AddCommand(newToolboxTouchTimestampCmd())
	cmd.AddCommand(newToolboxSHA256SumCmd())

	return cmd
}

func newToolboxJQCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "jq <expression>",
		Short: "Run a jq-compatible query against JSON input from stdin",
		Args:  cobra.ArbitraryArgs,
		Run: func(_ *cobra.Command, args []string) {
			os.Exit(toolbox.RunJQ(args, os.Stdin, os.Stdout, os.Stderr))
		},
	}
}

func newToolboxYQCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "yq <expression>",
		Short: "Run a yq-compatible query against YAML input from stdin",
		Args:  cobra.ArbitraryArgs,
		Run: func(_ *cobra.Command, args []string) {
			os.Exit(toolbox.RunYQ(args, os.Stdin, os.Stdout, os.Stderr))
		},
	}
}

func newToolboxTouchTimestampCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "touch-timestamp <file>",
		Short: "Write the current compact UTC timestamp to a file",
		Args:  cobra.ArbitraryArgs,
		Run: func(_ *cobra.Command, args []string) {
			os.Exit(toolbox.RunTouchTimestamp(args, os.Stdout, os.Stderr))
		},
	}
}

func newToolboxSHA256SumCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sha256sum [file]",
		Short: "Compute SHA256 hash in coreutils format",
		Args:  cobra.ArbitraryArgs,
		Run: func(_ *cobra.Command, args []string) {
			os.Exit(toolbox.RunSHA256Sum(args, os.Stdin, os.Stdout, os.Stderr))
		},
	}
}
