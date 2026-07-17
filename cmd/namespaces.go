package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var (
	namespacesListJSON bool
	namespacesListAll  bool
)

var namespacesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List namespaces",
	Long: `List namespaces reported by the server (GET <server>/namespaces).

By default only kagenti-enabled namespaces are listed; use --all to list every
namespace. With --json the raw JSON returned by the server is printed
unchanged, otherwise the namespaces are printed as a human-readable table.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newClient(cmd)
		if err != nil {
			return err
		}
		resp, err := client.ListNamespaces(cmd.Context(), !namespacesListAll)
		if err != nil {
			return err
		}

		if namespacesListJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(resp)
		}

		printNamespacesTable(cmd, resp.Namespaces)
		return nil
	},
}

func printNamespacesTable(cmd *cobra.Command, namespaces []string) {
	out := cmd.OutOrStdout()

	if len(namespaces) == 0 {
		fmt.Fprintln(out, "No namespaces found.")
		return
	}

	// Copy before sorting so we don't mutate the caller's slice, then print
	// one namespace per row under a header.
	sorted := append([]string(nil), namespaces...)
	sort.Strings(sorted)

	fmt.Fprintln(out, "NAME")
	for _, ns := range sorted {
		fmt.Fprintln(out, ns)
	}
}

func init() {
	namespacesCmd := newGroup("namespaces", "Manage namespaces")

	namespacesListCmd.Flags().BoolVar(&namespacesListJSON, "json", false, "print the raw JSON response unchanged")
	namespacesListCmd.Flags().BoolVar(&namespacesListAll, "all", false, "list all namespaces, not just kagenti-enabled ones")

	namespacesCmd.AddCommand(namespacesListCmd)
	rootCmd.AddCommand(namespacesCmd)
}
