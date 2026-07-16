package cmd

import "github.com/spf13/cobra"

// defaultImportNamespace is the namespace agents are imported into unless
// --namespace is given.
const defaultImportNamespace = "team1"

// newAgentsImportCmd builds the `agents import` command and its two
// subcommands, `from-image` and `from-source`. Flag values are bound to
// closure-local variables so each command owns its own state.
//
// The subcommands currently print UNIMPLEMENTED like the other stubs, but
// unlike newLeaf they define real flags (mirroring the backend's
// CreateAgentRequest fields), so the documented invocations parse correctly.
func newAgentsImportCmd() *cobra.Command {
	importCmd := newGroup("import", "Import an agent from an image or from source")

	importCmd.AddCommand(
		newAgentsImportFromImageCmd(),
		newAgentsImportFromSourceCmd(),
	)
	return importCmd
}

func newAgentsImportFromImageCmd() *cobra.Command {
	var (
		namespace       string
		name            string
		envVarsURL      string
		containerImage  string
		imagePullSecret string
	)

	cmd := &cobra.Command{
		Use:   "from-image",
		Short: unimplementedPrefix + "Import an agent from an existing container image",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return unimplementedRunE(cmd, nil)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&namespace, "namespace", "n", defaultImportNamespace, "namespace to import the agent into")
	f.StringVar(&name, "name", "", "name of the agent")
	f.StringVar(&envVarsURL, "envVarsURL", "", "URL to fetch environment variables from")
	f.StringVar(&containerImage, "containerImage", "", "container image to deploy")
	f.StringVar(&imagePullSecret, "imagePullSecret", "", "name of the image pull secret")

	return cmd
}

func newAgentsImportFromSourceCmd() *cobra.Command {
	var (
		namespace  string
		name       string
		envVarsURL string
		gitURL     string
		gitPath    string
		gitBranch  string
	)

	cmd := &cobra.Command{
		Use:   "from-source",
		Short: unimplementedPrefix + "Import an agent by building from source",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return unimplementedRunE(cmd, nil)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&namespace, "namespace", "n", defaultImportNamespace, "namespace to import the agent into")
	f.StringVar(&name, "name", "", "name of the agent")
	f.StringVar(&envVarsURL, "envVarsURL", "", "URL to fetch environment variables from")
	f.StringVar(&gitURL, "gitUrl", "", "git repository URL to build from")
	f.StringVar(&gitPath, "gitPath", "", "path within the git repository")
	f.StringVar(&gitBranch, "gitBranch", "main", "git branch to build from")

	return cmd
}
