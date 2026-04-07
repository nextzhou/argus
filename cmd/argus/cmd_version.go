package main

import "github.com/spf13/cobra"

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Printf("argus version %s\n", version)
		},
	}
}
