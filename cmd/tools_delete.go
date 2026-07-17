package cmd

import (
	"github.com/spf13/cobra"
)

var toolsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a tool",
	Long: `Delete a tool (DELETE <server>/tools/<namespace>/<name>), where
namespace is the --namespace flag if given, otherwise the current context's
namespace.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		namespace, err := toolsNamespace()
		if err != nil {
			return err
		}

		client, err := newClient(cmd)
		if err != nil {
			return err
		}
		resp, err := client.DeleteTool(cmd.Context(), namespace, name)
		if err != nil {
			return err
		}

		if resp.Message != "" {
			cmd.Println(resp.Message)
		} else {
			cmd.Printf("Tool %q deleted from namespace %q.\n", name, namespace)
		}
		return nil
	},
}
