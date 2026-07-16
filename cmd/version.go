package cmd

import (
	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/buildinfo"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		info := buildinfo.Info{Version: version, Commit: commit, Date: date}
		cmd.Println(info.String())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
