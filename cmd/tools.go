package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/apiclient"
)

var (
	toolsListJSON       bool
	toolsListNamespaces []string
)

var toolsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tools",
	Long: `List tools reported by the server (GET <server>/tools).

Use --namespaces to list tools across one or more namespaces; a separate
request (GET <server>/tools?namespace=<namespace>) is made for each, and the
results are combined. When --namespaces is omitted, the set of namespaces is
discovered from the server (GET <server>/namespaces) and tools are listed
across all of them.

By default the combined tools are printed as a single human-readable table
with a NAMESPACE column. With --json each namespace's raw response is printed
unchanged, separated by a line containing "---".`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newClient(cmd)
		if err != nil {
			return err
		}

		// When --namespaces is empty, discover the namespaces to query via
		// the same mechanism as `namespaces list` (GET /namespaces) and list
		// tools in each, rather than falling back to a single default-
		// namespace request.
		namespaces := toolsListNamespaces
		if len(namespaces) == 0 {
			nsResp, err := client.ListNamespaces(cmd.Context(), true)
			if err != nil {
				return err
			}
			namespaces = nsResp.Namespaces
		}

		responses := make([]*apiclient.ToolListResponse, 0, len(namespaces))
		for _, ns := range namespaces {
			resp, err := client.ListTools(cmd.Context(), ns)
			if err != nil {
				return err
			}
			responses = append(responses, resp)
		}

		if toolsListJSON {
			return printToolsJSON(cmd, responses)
		}

		// Combine all namespaces' tools into one table.
		var tools []apiclient.ToolSummary
		for _, resp := range responses {
			tools = append(tools, resp.Items...)
		}
		printToolsTable(cmd, tools)
		return nil
	},
}

// printToolsJSON prints each namespace's response as indented JSON, separated
// by a line containing "---".
func printToolsJSON(cmd *cobra.Command, responses []*apiclient.ToolListResponse) error {
	out := cmd.OutOrStdout()
	for i, resp := range responses {
		if i > 0 {
			fmt.Fprintln(out, "---")
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return nil
}

func printToolsTable(cmd *cobra.Command, tools []apiclient.ToolSummary) {
	out := cmd.OutOrStdout()

	if len(tools) == 0 {
		fmt.Fprintln(out, "No tools found.")
		return
	}

	// Stable ordering: namespace then name.
	sort.Slice(tools, func(i, j int) bool {
		if tools[i].Namespace != tools[j].Namespace {
			return tools[i].Namespace < tools[j].Namespace
		}
		return tools[i].Name < tools[j].Name
	})

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tNAMESPACE\tSTATUS\tWORKLOAD\tPROTOCOL\tDESCRIPTION")
	for _, tl := range tools {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			tl.Name,
			tl.Namespace,
			tl.Status,
			deref(tl.WorkloadType),
			strings.Join(tl.Labels.Protocol, ","),
			truncate(tl.Description),
		)
	}
	_ = w.Flush()
}

func init() {
	toolsCmd := newGroup("tools", "Manage tools")

	toolsListCmd.Flags().BoolVar(&toolsListJSON, "json", false, "print the raw JSON response unchanged")
	toolsListCmd.Flags().StringSliceVarP(&toolsListNamespaces, "namespaces", "n", nil, "namespaces to list tools in (repeatable or comma-separated; default: discovered)")

	toolsCmd.AddCommand(
		toolsListCmd,
		newToolsImportCmd(),
		newLeaf("delete [name]", "Delete a tool"),
	)
	rootCmd.AddCommand(toolsCmd)
}
