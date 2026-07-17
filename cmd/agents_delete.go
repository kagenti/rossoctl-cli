package cmd

import (
	"github.com/spf13/cobra"
)

var agentsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an agent",
	Long: `Delete an agent (DELETE <server>/agents/<namespace>/<name>), where
namespace is the --namespace flag if given, otherwise the current context's
namespace.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		namespace, err := agentsNamespace()
		if err != nil {
			return err
		}

		client, err := newClient(cmd)
		if err != nil {
			return err
		}
		resp, err := client.DeleteAgent(cmd.Context(), namespace, name)
		if err != nil {
			return err
		}

		if resp.Message != "" {
			cmd.Println(resp.Message)
		} else {
			cmd.Printf("Agent %q deleted from namespace %q.\n", name, namespace)
		}
		return nil
	},
}
