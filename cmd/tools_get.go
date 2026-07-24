package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/rossoctl/rossoctl-cli/internal/apiclient"
)

var toolsGetJSON bool

var toolsGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Show detailed information about a tool",
	Long: `Show detailed information about a single tool
(GET <server>/tools/<namespace>/<name>), where namespace is the namespace of
the current context.

By default the details are printed as single-column text, laid out in the same
sections as the web UI's tool detail page. With --json the raw JSON returned by
the server is printed unchanged.`,
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
		tool, err := client.GetTool(cmd.Context(), namespace, name)
		if err != nil {
			return err
		}

		if toolsGetJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(tool)
		}

		printToolDetail(cmd.OutOrStdout(), tool)
		return nil
	},
}

// printToolDetail renders a tool as single-column text, mirroring the section
// structure of the web UI's ToolDetailPage (header, Tool Information, Endpoint,
// Service, Status). The MCP Tools / MCP Inspector / YAML tabs are interactive
// and have no static equivalent, so they are omitted.
func printToolDetail(out io.Writer, t *apiclient.ToolDetail) {
	// Header: name, then ready status, protocols, workload type, and (when
	// simulated) the SIMULATED marker — the labels shown next to the title in
	// the UI.
	fmt.Fprintf(out, "%s\n", t.Metadata.Name)
	status := t.ReadyStatus
	if status == "" {
		status = "Not Ready"
	}
	fmt.Fprintf(out, "Status: %s\n", status)
	if protos := toolProtocols(t.Metadata.Labels); len(protos) > 0 {
		fmt.Fprintf(out, "Protocols: %s\n", strings.Join(protos, ", "))
	}
	fmt.Fprintf(out, "Workload Type: %s\n", workloadLabel(t))
	if isSimulatedLabels(t.Metadata.Labels) {
		fmt.Fprintln(out, "Simulated: yes")
	}

	// Tool Information.
	section(out, "Tool Information")
	rows := newRows()
	rows.add("Name", t.Metadata.Name)
	rows.add("Namespace", t.Metadata.Namespace)
	rows.add("Description", agentDescription(t))
	rows.add("Workload Type", workloadLabel(t))
	rows.add("Replicas", toolReplicaSummary(t))
	rows.add("Created", strDeref(t.Metadata.CreationTimestamp, "N/A"))
	rows.add("UID", strDeref(t.Metadata.UID, "N/A"))
	rows.flush(out)

	// Endpoint: the MCP Server URL. The UI prefers an external route when one
	// exists and otherwise shows the in-cluster URL; the CLI cannot check route
	// status, so it shows the in-cluster URL (the UI's safe default).
	section(out, "Endpoint")
	r := newRows()
	r.add("MCP Server URL", mcpInClusterURL(t))
	r.flush(out)

	// Service (when present).
	if t.Service != nil {
		section(out, "Service")
		s := newRows()
		s.add("Service Name", t.Service.Name)
		if t.Service.Type != "" {
			s.add("Type", t.Service.Type)
		}
		if t.Service.ClusterIP != "" {
			s.add("Cluster IP", t.Service.ClusterIP)
		}
		if len(t.Service.Ports) > 0 {
			var ports []string
			for _, p := range t.Service.Ports {
				label := ""
				if p.Name != "" {
					label = p.Name + ": "
				}
				port := fmt.Sprintf("%s%d", label, p.Port)
				if p.TargetPort != nil {
					port += fmt.Sprintf(" → %v", p.TargetPort)
				}
				ports = append(ports, port)
			}
			s.add("Ports", strings.Join(ports, ", "))
		}
		s.flush(out)
	}

	// Status conditions.
	conds := conditions(t)
	section(out, "Status")
	if len(conds) == 0 {
		fmt.Fprintln(out, "  No status conditions available.")
		return
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  TYPE\tSTATUS\tREASON\tMESSAGE\tLAST TRANSITION")
	for _, c := range conds {
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n",
			mapString(c, "type"),
			mapString(c, "status"),
			orDefault(mapString(c, "reason"), "-"),
			orDefault(mapString(c, "message"), "-"),
			orDefault(firstMapString(c, "lastTransitionTime", "last_transition_time"), "-"),
		)
	}
	_ = w.Flush()
}

// --- tool-specific helpers ---

// workloadLabel renders the workload type the way the UI labels it:
// "StatefulSet" for statefulset, "Deployment" otherwise.
func workloadLabel(t *apiclient.ToolDetail) string {
	if workloadType(t) == "statefulset" {
		return "StatefulSet"
	}
	return "Deployment"
}

// toolReplicaSummary renders "ready/desired" as the UI's Replicas field does.
func toolReplicaSummary(t *apiclient.ToolDetail) string {
	desired := mapInt(t.Spec, "replicas", 1)
	ready := firstMapInt(t.Status, 0, "readyReplicas", "ready_replicas")
	return fmt.Sprintf("%d/%d", ready, desired)
}

// mcpInClusterURL builds the in-cluster MCP server URL the UI shows, using the
// first service port (falling back to 8000, matching the UI).
func mcpInClusterURL(t *apiclient.ToolDetail) string {
	port := 8000
	if t.Service != nil && len(t.Service.Ports) > 0 && t.Service.Ports[0].Port != 0 {
		port = t.Service.Ports[0].Port
	}
	return fmt.Sprintf("http://%s-mcp.%s.svc.cluster.local:%d/mcp",
		t.Metadata.Name, t.Metadata.Namespace, port)
}

// toolProtocols returns the protocols shown as labels in the UI header,
// defaulting to MCP when none are declared (as the UI does).
func toolProtocols(labels map[string]string) []string {
	protos := protocolsFromLabels(labels)
	if len(protos) == 0 {
		protos = []string{"MCP"}
	}
	return protos
}

// isSimulatedLabels reports whether labels mark the tool as simulated, matching
// the UI's isSimulatedLabels helper.
func isSimulatedLabels(labels map[string]string) bool {
	return labels["rossoctl.io/simulated"] == "true"
}

func init() {
	toolsGetCmd.Flags().BoolVar(&toolsGetJSON, "json", false, "print the raw JSON response unchanged")
}
