package main

import "github.com/spf13/cobra"

type versionOutput struct {
	Version string `json:"version"`
}

func newVersionCmd(version string) *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, _ []string) {
			if jsonFlag {
				_ = writeJSONOK(cmd, versionOutput{Version: version})
				return
			}

			cmd.Printf("argus version %s\n", version)
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}
