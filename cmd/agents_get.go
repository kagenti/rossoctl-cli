package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/apiclient"
)

var agentsGetJSON bool

var agentsGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Show detailed information about an agent",
	Long: `Show detailed information about a single agent
(GET <server>/agents/<namespace>/<name>), where namespace is the namespace of
the current context.

By default the details are printed as single-column text, laid out in the same
sections as the web UI's agent detail page. With --json the raw JSON returned
by the server is printed unchanged.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		namespace, err := currentNamespace()
		if err != nil {
			return err
		}

		client, err := newClient(cmd)
		if err != nil {
			return err
		}
		agent, err := client.GetAgent(cmd.Context(), namespace, name)
		if err != nil {
			return err
		}

		if agentsGetJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(agent)
		}

		printAgentDetail(cmd.OutOrStdout(), agent)
		return nil
	},
}

// printAgentDetail renders an agent as single-column text, mirroring the
// section structure of the web UI's AgentDetailPage (header, Agent
// Information, Endpoint, Source, Status).
func printAgentDetail(out io.Writer, a *apiclient.AgentDetail) {
	// Header: name and ready status, then protocols.
	fmt.Fprintf(out, "%s\n", a.Metadata.Name)
	status := a.ReadyStatus
	if status == "" {
		status = "Unknown"
	}
	fmt.Fprintf(out, "Status: %s\n", status)
	if protos := protocolsFromLabels(a.Metadata.Labels); len(protos) > 0 {
		fmt.Fprintf(out, "Protocols: %s\n", strings.Join(protos, ", "))
	}

	// Agent Information.
	section(out, "Agent Information")
	rows := newRows()
	rows.add("Name", a.Metadata.Name)
	rows.add("Namespace", a.Metadata.Namespace)
	rows.add("Description", agentDescription(a))
	rows.add("Workload Type", title(workloadType(a)))
	if wt := workloadType(a); wt != "job" {
		rows.add("Replicas", replicaSummary(a))
	}
	rows.add("Created", strDeref(a.Metadata.CreationTimestamp, "N/A"))
	rows.add("UID", strDeref(a.Metadata.UID, "N/A"))
	rows.flush(out)

	// Endpoint (Service info, when present).
	if a.Service != nil {
		section(out, "Endpoint")
		r := newRows()
		r.add("Service", fmt.Sprintf("%s (%s)", a.Service.Name, orDefault(a.Service.Type, "ClusterIP")))
		r.add("Cluster IP", orDefault(a.Service.ClusterIP, "N/A"))
		if len(a.Service.Ports) > 0 {
			var ports []string
			for _, p := range a.Service.Ports {
				label := ""
				if p.Name != "" {
					label = p.Name + ": "
				}
				ports = append(ports, fmt.Sprintf("%s%d→%v", label, p.Port, p.TargetPort))
			}
			r.add("Ports", strings.Join(ports, ", "))
		}
		r.flush(out)
	}

	// Source (when spec.source.git is present).
	if git := gitSource(a); git != nil {
		section(out, "Source")
		r := newRows()
		r.add("Git URL", mapString(git, "url"))
		r.add("Path", orDefault(mapString(git, "path"), "/"))
		r.add("Branch", orDefault(mapString(git, "branch"), "main"))
		if tag := imageTag(a); tag != "" {
			r.add("Image Tag", tag)
		}
		r.flush(out)
	}

	// Status conditions.
	conds := conditions(a)
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

// --- small rendering helpers ---

// rows accumulates aligned "Term: value" lines and flushes them through a
// tabwriter so the values line up in a single column.
type rows struct{ pairs [][2]string }

func newRows() *rows { return &rows{} }

func (r *rows) add(term, value string) { r.pairs = append(r.pairs, [2]string{term, value}) }

func (r *rows) flush(out io.Writer) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, p := range r.pairs {
		fmt.Fprintf(w, "  %s:\t%s\n", p[0], p[1])
	}
	_ = w.Flush()
}

func section(out io.Writer, name string) {
	fmt.Fprintf(out, "\n%s\n", name)
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func strDeref(s *string, def string) string {
	if s == nil || *s == "" {
		return def
	}
	return *s
}

// --- agent field extraction (spec/status are free-form maps) ---

func workloadType(a *apiclient.AgentDetail) string {
	if a.WorkloadType != "" {
		return a.WorkloadType
	}
	if wt := a.Metadata.Labels["kagenti.io/workload-type"]; wt != "" {
		return wt
	}
	return "deployment"
}

func agentDescription(a *apiclient.AgentDetail) string {
	if d := mapString(a.Spec, "description"); d != "" {
		return d
	}
	if d := a.Metadata.Annotations["kagenti.io/description"]; d != "" {
		return d
	}
	return "No description available"
}

func replicaSummary(a *apiclient.AgentDetail) string {
	replicas := mapInt(a.Spec, "replicas", 1)
	ready := firstMapInt(a.Status, 0, "readyReplicas", "ready_replicas")
	available := firstMapInt(a.Status, 0, "availableReplicas", "available_replicas")
	s := fmt.Sprintf("%d/%d ready", ready, replicas)
	if available > 0 {
		s += fmt.Sprintf(" (%d available)", available)
	}
	return s
}

// gitSource returns spec.source.git as a map, or nil when absent.
func gitSource(a *apiclient.AgentDetail) map[string]any {
	source, ok := a.Spec["source"].(map[string]any)
	if !ok {
		return nil
	}
	git, ok := source["git"].(map[string]any)
	if !ok {
		return nil
	}
	return git
}

func imageTag(a *apiclient.AgentDetail) string {
	image, ok := a.Spec["image"].(map[string]any)
	if !ok {
		return ""
	}
	return mapString(image, "tag")
}

// conditions returns status.conditions as a slice of maps (empty when absent).
func conditions(a *apiclient.AgentDetail) []map[string]any {
	raw, ok := a.Status["conditions"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func protocolsFromLabels(labels map[string]string) []string {
	const prefix = "protocol.kagenti.io/"
	var protos []string
	for k := range labels {
		if strings.HasPrefix(k, prefix) && len(k) > len(prefix) {
			protos = append(protos, strings.ToUpper(k[len(prefix):]))
		}
	}
	if len(protos) == 0 {
		if p := labels["kagenti.io/protocol"]; p != "" {
			protos = append(protos, strings.ToUpper(p))
		}
	}
	sort.Strings(protos)
	return protos
}

// --- generic map readers for free-form JSON ---

func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func firstMapString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := mapString(m, k); v != "" {
			return v
		}
	}
	return ""
}

// mapInt reads an integer-valued key. JSON numbers decode as float64, so both
// float64 and int are handled.
func mapInt(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return def
	}
}

func firstMapInt(m map[string]any, def int, keys ...string) int {
	for _, k := range keys {
		if _, present := m[k]; present {
			return mapInt(m, k, def)
		}
	}
	return def
}

func init() {
	agentsGetCmd.Flags().BoolVar(&agentsGetJSON, "json", false, "print the raw JSON response unchanged")
}
