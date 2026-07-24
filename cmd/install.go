package cmd

import "github.com/spf13/cobra"

// installText is the guidance printed by `rossoctl install`. Installation is a
// scripted, out-of-band process (clone the repo, run a setup script for your
// cluster type), so the command prints instructions rather than performing it.
const installText = `To install the K8s API version, obtain the source code and

git clone https://github.com/rossoctl/rossoctl.git
cd rossoctl

Then do one of the following
./scripts/kind/setup-rossoctl.sh
./scripts/k8s/setup-rossoctl.sh
./scripts/ocp/setup-rossoctl.sh

To run Cortex version, use ` + "`rossoctl cortex`"

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Show how to install the Rossoctl platform",
	Long: `Print instructions for installing Rossoctl.

The K8s API version is installed from source with a per-cluster-type setup
script; the Cortex version is run directly via ` + "`rossoctl cortex`" + `.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.Println(installText)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
