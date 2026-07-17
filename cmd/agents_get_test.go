package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// agentDetailBody mimics the backend GET /agents/{ns}/{name} response for a
// source-built deployment agent with a service and status conditions.
const agentDetailBody = `{
	"metadata": {
		"name": "orders",
		"namespace": "team1",
		"labels": {"protocol.kagenti.io/a2a": "true", "kagenti.io/workload-type": "deployment"},
		"annotations": {"kagenti.io/description": "Handles orders"},
		"creationTimestamp": "2026-01-02T03:04:05Z",
		"uid": "abc-123"
	},
	"spec": {
		"replicas": 2,
		"source": {"git": {"url": "https://github.com/x/y", "path": "agents/orders", "branch": "dev"}},
		"image": {"tag": "v0.0.1"}
	},
	"status": {
		"readyReplicas": 2,
		"availableReplicas": 2,
		"conditions": [
			{"type": "Available", "status": "True", "reason": "MinimumReplicasAvailable",
			 "message": "Deployment has minimum availability.", "lastTransitionTime": "2026-01-02T03:05:00Z"}
		]
	},
	"workloadType": "deployment",
	"readyStatus": "Ready",
	"service": {
		"name": "orders",
		"type": "ClusterIP",
		"clusterIP": "10.0.0.5",
		"ports": [{"name": "http", "port": 8080, "targetPort": 8000}]
	}
}`

// setupAgentGetContext points the current context at srv with namespace team1.
func setupAgentGetContext(t *testing.T, srv *httptest.Server) {
	t.Helper()
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	// set-context validates against /namespaces; the mock returns team1.
	if _, err := execute(t, "config", "set-context", "--namespace", "team1"); err != nil {
		t.Fatalf("set-context: %v", err)
	}
}

func newAgentGetServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces":
			_, _ = w.Write([]byte(`{"namespaces":["team1"]}`))
		case "/api/v1/agents/team1/orders":
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

func TestAgentsGetText(t *testing.T) {
	isolateHome(t)
	srv := newAgentGetServer(t, agentDetailBody, 0)
	setupAgentGetContext(t, srv)

	out, err := execute(t, "agents", "get", "orders")
	if err != nil {
		t.Fatalf("agents get: %v", err)
	}

	// Section headers and key fields, mirroring the UI's detail page.
	for _, want := range []string{
		"orders",
		"Status: Ready",
		"Protocols: A2A",
		"Agent Information",
		"Namespace:", "team1",
		"Description:", "Handles orders",
		"Workload Type:", "Deployment",
		"Replicas:", "2/2 ready (2 available)",
		"Created:", "2026-01-02T03:04:05Z",
		"UID:", "abc-123",
		"Endpoint",
		"Service:", "orders (ClusterIP)",
		"Cluster IP:", "10.0.0.5",
		"Ports:", "http: 8080→8000",
		"Source",
		"Git URL:", "https://github.com/x/y",
		"Path:", "agents/orders",
		"Branch:", "dev",
		"Image Tag:", "v0.0.1",
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

func TestAgentsGetJSON(t *testing.T) {
	isolateHome(t)
	srv := newAgentGetServer(t, agentDetailBody, 0)
	setupAgentGetContext(t, srv)

	out, err := execute(t, "agents", "get", "orders", "--json")
	if err != nil {
		t.Fatalf("agents get --json: %v", err)
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
	if decoded.Metadata.Name != "orders" || decoded.ReadyStatus != "Ready" {
		t.Errorf("unexpected decoded JSON: %+v", decoded)
	}
}

func TestAgentsGetRequiresNamespace(t *testing.T) {
	isolateHome(t)
	// Context has no namespace set.
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", "http://x/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	if _, err := execute(t, "agents", "get", "orders"); err == nil {
		t.Error("agents get should error when the current context has no namespace")
	}
}

func TestAgentsGetMinimalAgent(t *testing.T) {
	isolateHome(t)
	// No service, no source, no conditions — the renderer must still work.
	body := `{
		"metadata": {"name": "orders", "namespace": "team1", "labels": {}, "annotations": {}},
		"spec": {},
		"status": {},
		"workloadType": "deployment",
		"readyStatus": "Not Ready"
	}`
	srv := newAgentGetServer(t, body, 0)
	setupAgentGetContext(t, srv)

	out, err := execute(t, "agents", "get", "orders")
	if err != nil {
		t.Fatalf("agents get: %v", err)
	}
	for _, want := range []string{
		"Status: Not Ready",
		"No description available",
		"Created:", "N/A",
		"No status conditions available",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("minimal output missing %q:\n%s", want, out)
		}
	}
	// No Endpoint/Source sections when the data is absent.
	if strings.Contains(out, "Endpoint") {
		t.Errorf("Endpoint section should be omitted when no service:\n%s", out)
	}
	if strings.Contains(out, "Source") {
		t.Errorf("Source section should be omitted when no git source:\n%s", out)
	}
}
