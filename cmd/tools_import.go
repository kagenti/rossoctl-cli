package cmd

import "github.com/spf13/cobra"

// defaultImportNamespace is the namespace the (stub) tools import subcommands
// default to unless --namespace is given.
const defaultImportNamespace = "team1"

// newToolsImportCmd builds the `tools import` command and its two subcommands,
// `from-image` and `from-source`, mirroring `agents import`. Flag values are
// bound to closure-local variables so each command owns its own state.
//
// The subcommands currently print UNIMPLEMENTED like the other stubs, but
// unlike newLeaf they define real flags (mirroring the backend's
// CreateToolRequest fields), so the documented invocations parse correctly.
func newToolsImportCmd() *cobra.Command {
	importCmd := newGroup("import", "Import a tool from an image or from source")

	importCmd.AddCommand(
		newToolsImportFromImageCmd(),
		newToolsImportFromSourceCmd(),
	)
	return importCmd
}

func newToolsImportFromImageCmd() *cobra.Command {
	var (
		namespace       string
		name            string
		envVarsURL      string
		containerImage  string
		imagePullSecret string
	)

	cmd := &cobra.Command{
		Use:   "from-image",
		Short: unimplementedPrefix + "Import a tool from an existing container image",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return unimplementedRunE(cmd, nil)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&namespace, "namespace", "n", defaultImportNamespace, "namespace to import the tool into")
	f.StringVar(&name, "name", "", "name of the tool")
	f.StringVar(&envVarsURL, "envVarsURL", "", "URL to fetch environment variables from")
	f.StringVar(&containerImage, "containerImage", "", "container image to deploy")
	f.StringVar(&imagePullSecret, "imagePullSecret", "", "name of the image pull secret")

	return cmd
}

func newToolsImportFromSourceCmd() *cobra.Command {
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
		Short: unimplementedPrefix + "Import a tool by building from source",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return unimplementedRunE(cmd, nil)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&namespace, "namespace", "n", defaultImportNamespace, "namespace to import the tool into")
	f.StringVar(&name, "name", "", "name of the tool")
	f.StringVar(&envVarsURL, "envVarsURL", "", "URL to fetch environment variables from")
	f.StringVar(&gitURL, "gitUrl", "", "git repository URL to build from")
	f.StringVar(&gitPath, "gitPath", "", "path within the git repository")
	f.StringVar(&gitBranch, "gitBranch", "main", "git branch to build from")

	return cmd
}
