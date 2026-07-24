package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// toolDetailBody mimics the backend GET /tools/{ns}/{name} response for a
// deployment tool with a service and status conditions.
const toolDetailBody = `{
	"metadata": {
		"name": "weather-mcp",
		"namespace": "team1",
		"labels": {"protocol.rossoctl.io/mcp": "true"},
		"annotations": {"rossoctl.io/description": "Weather MCP server"},
		"creationTimestamp": "2026-01-02T03:04:05Z",
		"uid": "tool-123"
	},
	"spec": {"replicas": 1},
	"status": {
		"readyReplicas": 1,
		"conditions": [
			{"type": "Available", "status": "True", "reason": "MinimumReplicasAvailable",
			 "message": "Deployment has minimum availability.", "lastTransitionTime": "2026-01-02T03:05:00Z"}
		]
	},
	"workloadType": "deployment",
	"readyStatus": "Ready",
	"service": {
		"name": "weather-mcp",
		"type": "ClusterIP",
		"clusterIP": "10.0.0.9",
		"ports": [{"name": "mcp", "port": 8000, "targetPort": 8000}]
	}
}`

// setupToolGetContext points the current context at srv with namespace team1.
func setupToolGetContext(t *testing.T, srv *httptest.Server) {
	t.Helper()
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	if _, err := execute(t, "config", "set-context", "--namespace", "team1"); err != nil {
		t.Fatalf("set-context: %v", err)
	}
}

func newToolGetServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces":
			_, _ = w.Write([]byte(`{"namespaces":["team1"]}`))
		case "/api/v1/tools/team1/weather-mcp":
			if status != 0 {
				w.WriteHeader(status)
			}
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestToolsGetText(t *testing.T) {
	isolateHome(t)
	srv := newToolGetServer(t, toolDetailBody, 0)
	setupToolGetContext(t, srv)

	out, err := execute(t, "tools", "get", "weather-mcp")
	if err != nil {
		t.Fatalf("tools get: %v", err)
	}

	// Section headers and key fields, mirroring the UI's tool detail page.
	for _, want := range []string{
		"weather-mcp",
		"Status: Ready",
		"Protocols: MCP",
		"Workload Type: Deployment",
		"Tool Information",
		"Namespace:", "team1",
		"Description:", "Weather MCP server",
		"Replicas:", "1/1",
		"Created:", "2026-01-02T03:04:05Z",
		"UID:", "tool-123",
		"Endpoint",
		"MCP Server URL:", "http://weather-mcp-mcp.team1.svc.cluster.local:8000/mcp",
		"Service",
		"Service Name:", "weather-mcp",
		"Type:", "ClusterIP",
		"Cluster IP:", "10.0.0.9",
		"Ports:", "mcp: 8000 → 8000",
		"Status",
		"Available", "True", "MinimumReplicasAvailable",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q:\n%s", want, out)
		}
	}

	// It must not be raw JSON.
	if strings.Contains(out, "\"metadata\"") {
		t.Errorf("text output unexpectedly contains raw JSON:\n%s", out)
	}
}

func TestToolsGetJSON(t *testing.T) {
	isolateHome(t)
	srv := newToolGetServer(t, toolDetailBody, 0)
	setupToolGetContext(t, srv)

	out, err := execute(t, "tools", "get", "weather-mcp", "--json")
	if err != nil {
		t.Fatalf("tools get --json: %v", err)
	}
	var decoded struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		ReadyStatus string `json:"readyStatus"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	if decoded.Metadata.Name != "weather-mcp" || decoded.ReadyStatus != "Ready" {
		t.Errorf("unexpected decoded JSON: %+v", decoded)
	}
}

func TestToolsGetNamespaceOverride(t *testing.T) {
	isolateHome(t)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces" {
			_, _ = w.Write([]byte(`{"namespaces":["team1","team2"]}`))
			return
		}
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"metadata":{"name":"weather-mcp","namespace":"team2"},"spec":{},"status":{},"workloadType":"deployment","readyStatus":"Ready"}`))
	}))
	t.Cleanup(srv.Close)

	setupToolGetContext(t, srv)

	// --namespace team2 must override the context, hitting /tools/team2/weather-mcp.
	if _, err := execute(t, "tools", "--namespace", "team2", "get", "weather-mcp"); err != nil {
		t.Fatalf("tools get: %v", err)
	}
	if gotPath != "/api/v1/tools/team2/weather-mcp" {
		t.Errorf("requested path = %q, want /api/v1/tools/team2/weather-mcp", gotPath)
	}
}

func TestToolsGetRequiresNamespace(t *testing.T) {
	isolateHome(t)
	// Context has no namespace set.
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", "http://x/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	if _, err := execute(t, "tools", "get", "weather-mcp"); err == nil {
		t.Error("tools get should error when the current context has no namespace")
	}
}

func TestToolsGetMinimalTool(t *testing.T) {
	isolateHome(t)
	// No service, no conditions, no labels — the renderer must still work and
	// fall back to the default MCP protocol and port 8000.
	body := `{
		"metadata": {"name": "weather-mcp", "namespace": "team1", "labels": {}, "annotations": {}},
		"spec": {},
		"status": {},
		"workloadType": "statefulset",
		"readyStatus": "Not Ready"
	}`
	srv := newToolGetServer(t, body, 0)
	setupToolGetContext(t, srv)

	out, err := execute(t, "tools", "get", "weather-mcp")
	if err != nil {
		t.Fatalf("tools get: %v", err)
	}
	for _, want := range []string{
		"Status: Not Ready",
		"Protocols: MCP",
		"Workload Type: StatefulSet",
		"No description available",
		"Replicas:", "0/1",
		"MCP Server URL:", "http://weather-mcp-mcp.team1.svc.cluster.local:8000/mcp",
		"No status conditions available",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("minimal output missing %q:\n%s", want, out)
		}
	}
	// No Service section when the data is absent.
	if strings.Contains(out, "Service Name") {
		t.Errorf("Service section should be omitted when no service:\n%s", out)
	}
}
