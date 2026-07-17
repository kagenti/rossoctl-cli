package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/apiclient"
)

// toolsImportDeploymentType backs the persistent --deployment-type flag on the
// tools import group, inherited by from-image and from-source. It maps to the
// backend's workloadType.
var toolsImportDeploymentType string

// newToolsImportCmd builds the `tools import` command and its two subcommands,
// `from-image` and `from-source`, mirroring `agents import`.
//
// The namespace for the created tool comes from the tools group's --namespace
// flag (or the current context), via toolsNamespace().
func newToolsImportCmd() *cobra.Command {
	importCmd := newGroup("import", "Import a tool from an image or from source")

	// Persistent so both subcommands inherit it. Tools support deployment and
	// statefulset workload types.
	importCmd.PersistentFlags().StringVar(&toolsImportDeploymentType, "deployment-type", "deployment",
		"workload type for the tool: deployment|statefulset")

	importCmd.AddCommand(
		newToolsImportFromImageCmd(),
		newToolsImportFromSourceCmd(),
	)
	return importCmd
}

func newToolsImportFromImageCmd() *cobra.Command {
	var (
		name            string
		envVarsURL      string
		containerImage  string
		imagePullSecret string
	)

	cmd := &cobra.Command{
		Use:   "from-image",
		Short: "Import a tool from an existing container image",
		Long: `Import a tool from an existing container image (POST <server>/tools).

The tool is created in the namespace from the tools --namespace flag, or the
current context's namespace. --deployment-type selects the workload type. Env
vars are fetched from --envVarsURL (newline-separated key=value pairs).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if containerImage == "" {
				return fmt.Errorf("--containerImage is required")
			}

			namespace, err := toolsNamespace()
			if err != nil {
				return err
			}

			envVars, err := fetchEnvVars(cmd.Context(), cmd, envVarsURL)
			if err != nil {
				return err
			}

			client, err := newClient(cmd)
			if err != nil {
				return err
			}
			resp, err := client.CreateTool(cmd.Context(), &apiclient.CreateToolRequest{
				Name:             name,
				Namespace:        namespace,
				DeploymentMethod: "image",
				WorkloadType:     toolsImportDeploymentType,
				ContainerImage:   containerImage,
				ImagePullSecret:  imagePullSecret,
				EnvVars:          envVars,
			})
			if err != nil {
				return err
			}

			printCreateToolResult(cmd, resp, name, namespace)
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&name, "name", "", "name of the tool (required)")
	f.StringVar(&envVarsURL, "envVarsURL", "", "URL to fetch environment variables from (newline-separated key=value)")
	f.StringVar(&containerImage, "containerImage", "", "container image to deploy (required)")
	f.StringVar(&imagePullSecret, "imagePullSecret", "", "name of the image pull secret")

	return cmd
}

func newToolsImportFromSourceCmd() *cobra.Command {
	var (
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
	f.StringVar(&name, "name", "", "name of the tool")
	f.StringVar(&envVarsURL, "envVarsURL", "", "URL to fetch environment variables from")
	f.StringVar(&gitURL, "gitUrl", "", "git repository URL to build from")
	f.StringVar(&gitPath, "gitPath", "", "path within the git repository")
	f.StringVar(&gitBranch, "gitBranch", "main", "git branch to build from")

	return cmd
}

// printCreateToolResult reports the outcome of a create request, preferring
// the server's message.
func printCreateToolResult(cmd *cobra.Command, resp *apiclient.CreateToolResponse, name, namespace string) {
	if resp.Message != "" {
		cmd.Println(resp.Message)
		return
	}
	cmd.Printf("Tool %q created in namespace %q.\n", name, namespace)
}
