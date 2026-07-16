package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const agentsBody = `{
	"items": [
		{
			"name": "orders-agent",
			"namespace": "team1",
			"description": "Handles orders",
			"status": "Ready",
			"labels": {"protocol": ["a2a"], "framework": "LangGraph", "type": "agent"},
			"workloadType": "deployment",
			"createdAt": "2026-01-02T03:04:05Z"
		},
		{
			"name": "weather",
			"namespace": "team1",
			"description": "Weather agent",
			"status": "Not Ready",
			"labels": {"protocol": null, "framework": null, "type": "agent"},
			"workloadType": null,
			"createdAt": null
		}
	]
}`

// newAgentsServer serves both /agents (returning body) and /namespaces
// (returning a single "default" namespace, used when the command discovers
// namespaces because --namespaces was omitted). The returned pointer captures
// the RawQuery of the most recent /agents request.
func newAgentsServer(t *testing.T, body string) (*httptest.Server, *string) {
	t.Helper()
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces":
			_, _ = w.Write([]byte(`{"namespaces": ["default"]}`))
		case "/api/v1/agents":
			gotQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &gotQuery
}

func TestAgentsListTable(t *testing.T) {
	srv, _ := newAgentsServer(t, agentsBody)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "agents", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"NAME", "NAMESPACE", "STATUS", "WORKLOAD", "PROTOCOL", "DESCRIPTION",
		"orders-agent", "team1", "Ready", "deployment", "a2a", "Handles orders",
		"weather", "Not Ready",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}

	// The nil workloadType/protocol should render as "-", not "<nil>".
	if strings.Contains(out, "<nil>") {
		t.Errorf("table rendered a nil pointer:\n%s", out)
	}
	// Human output must not be raw JSON.
	if strings.Contains(out, "\"items\"") {
		t.Errorf("human output unexpectedly contains raw JSON:\n%s", out)
	}
}

func TestAgentsListTableTruncatesDescription(t *testing.T) {
	long := "This description is definitely longer than thirty characters"
	srv, _ := newAgentsServer(t, `{"items":[{"name":"a","namespace":"team1","description":"`+long+
		`","status":"Ready","labels":{"protocol":null,"framework":null,"type":"agent"},"workloadType":null,"createdAt":null}]}`)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "agents", "list", "-n", "team1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := long[:27] + "..."
	if !strings.Contains(out, want) {
		t.Errorf("table missing truncated description %q:\n%s", want, out)
	}
	// The full description must not appear.
	if strings.Contains(out, long) {
		t.Errorf("table contains untruncated description:\n%s", out)
	}
}

func TestAgentsListJSON(t *testing.T) {
	srv, _ := newAgentsServer(t, agentsBody)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "agents", "list", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	if len(decoded.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(decoded.Items))
	}
}

func TestAgentsListVerboseLogsToStderr(t *testing.T) {
	srv, _ := newAgentsServer(t, `{"items": []}`)

	stdout, stderr, err := executeSplit(t,
		"--verbose", "--server", srv.URL+"/api/v1/", "agents", "list", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The request line must be logged to stderr, including method and URL.
	if !strings.Contains(stderr, "GET "+srv.URL+"/api/v1/agents") {
		t.Errorf("stderr missing request log:\n%s", stderr)
	}
	// And the response status.
	if !strings.Contains(stderr, "200 OK") {
		t.Errorf("stderr missing response log:\n%s", stderr)
	}
	// stdout must stay clean JSON — no log noise mixed in.
	if strings.Contains(stdout, "GET ") {
		t.Errorf("stdout contains verbose log output:\n%s", stdout)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("stdout is not clean JSON with --verbose: %v\n%s", err, stdout)
	}
}

func TestAgentsListNoVerboseNoLog(t *testing.T) {
	srv, _ := newAgentsServer(t, `{"items": []}`)

	_, stderr, err := executeSplit(t, "--server", srv.URL+"/api/v1/", "agents", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stderr, "GET ") {
		t.Errorf("expected no request logging without --verbose, got:\n%s", stderr)
	}
}

func TestAgentsListNamespacesFlag(t *testing.T) {
	srv, gotQuery := newAgentsServer(t, `{"items": []}`)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "agents", "list", "--namespaces", "team1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *gotQuery != "namespace=team1" {
		t.Errorf("query = %q, want %q", *gotQuery, "namespace=team1")
	}
	if !strings.Contains(out, "No agents found") {
		t.Errorf("empty list output = %q, want %q", out, "No agents found")
	}
}

// newPerNamespaceAgentsServer returns a server that responds based on the
// requested namespace, recording every namespace it was queried for in order.
func newPerNamespaceAgentsServer(t *testing.T, byNamespace map[string]string) (*httptest.Server, *[]string) {
	t.Helper()
	var queried []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		ns := r.URL.Query().Get("namespace")
		queried = append(queried, ns)
		body, ok := byNamespace[ns]
		if !ok {
			body = `{"items": []}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &queried
}

func TestAgentsListDiscoversNamespacesWhenEmpty(t *testing.T) {
	var agentsQueried []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces":
			// enabled-only discovery => no enabled_only=false query param.
			if r.URL.RawQuery != "" {
				t.Errorf("namespaces query = %q, want empty (enabled-only)", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"namespaces": ["alpha", "beta"]}`))
		case "/api/v1/agents":
			ns := r.URL.Query().Get("namespace")
			agentsQueried = append(agentsQueried, ns)
			_, _ = w.Write([]byte(`{"items":[{"name":"a-` + ns + `","namespace":"` + ns +
				`","description":"d","status":"Ready","labels":{"protocol":null,"framework":null,"type":"agent"},"workloadType":null,"createdAt":null}]}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	// No --namespaces: the command must discover [alpha, beta] and query each.
	out, err := execute(t, "--server", srv.URL+"/api/v1/", "agents", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(agentsQueried) != 2 || agentsQueried[0] != "alpha" || agentsQueried[1] != "beta" {
		t.Errorf("agents queried for namespaces %v, want [alpha beta]", agentsQueried)
	}
	for _, want := range []string{"a-alpha", "alpha", "a-beta", "beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestAgentsListDiscoversNoNamespaces(t *testing.T) {
	agentsCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces":
			_, _ = w.Write([]byte(`{"namespaces": []}`))
		case "/api/v1/agents":
			agentsCalled = true
			_, _ = w.Write([]byte(`{"items": []}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "agents", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No discovered namespaces => no /agents requests at all.
	if agentsCalled {
		t.Error("expected no /agents request when no namespaces are discovered")
	}
	if !strings.Contains(out, "No agents found") {
		t.Errorf("output = %q, want %q", out, "No agents found")
	}
}

func TestAgentsListMultipleNamespacesTable(t *testing.T) {
	srv, queried := newPerNamespaceAgentsServer(t, map[string]string{
		"team1": `{"items":[{"name":"orders","namespace":"team1","description":"d1","status":"Ready","labels":{"protocol":["a2a"],"framework":null,"type":"agent"},"workloadType":"deployment","createdAt":null}]}`,
		"team2": `{"items":[{"name":"weather","namespace":"team2","description":"d2","status":"Not Ready","labels":{"protocol":null,"framework":null,"type":"agent"},"workloadType":null,"createdAt":null}]}`,
	})

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "agents", "list", "--namespaces", "team1,team2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// One request per namespace, in order.
	if len(*queried) != 2 || (*queried)[0] != "team1" || (*queried)[1] != "team2" {
		t.Errorf("queried namespaces = %v, want [team1 team2]", *queried)
	}

	// Both namespaces' agents appear in the single combined table.
	for _, want := range []string{"orders", "team1", "weather", "team2"} {
		if !strings.Contains(out, want) {
			t.Errorf("combined table missing %q:\n%s", want, out)
		}
	}
	// One header only (single table).
	if n := strings.Count(out, "NAMESPACE"); n != 1 {
		t.Errorf("expected exactly one table header, found %d:\n%s", n, out)
	}
}

func TestAgentsListMultipleNamespacesJSON(t *testing.T) {
	srv, queried := newPerNamespaceAgentsServer(t, map[string]string{
		"team1": `{"items":[{"name":"orders","namespace":"team1","description":"d","status":"Ready","labels":{"protocol":null,"framework":null,"type":"agent"},"workloadType":null,"createdAt":null}]}`,
		"team2": `{"items":[]}`,
	})

	out, err := execute(t, "--server", srv.URL+"/api/v1/",
		"agents", "list", "--json", "--namespaces", "team1", "--namespaces", "team2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Repeated --namespaces flags accumulate, one request each.
	if len(*queried) != 2 {
		t.Fatalf("expected 2 requests, got %d (%v)", len(*queried), *queried)
	}

	// Two JSON documents separated by a "---" line.
	parts := strings.Split(out, "---\n")
	if len(parts) != 2 {
		t.Fatalf("expected 2 JSON parts separated by ---, got %d:\n%s", len(parts), out)
	}
	for i, part := range parts {
		var decoded struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal([]byte(part), &decoded); err != nil {
			t.Errorf("part %d is not valid JSON: %v\n%s", i, err, part)
		}
	}
}

func TestAgentsListSingleJSONHasNoSeparator(t *testing.T) {
	srv, _ := newAgentsServer(t, `{"items": []}`)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "agents", "list", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "---") {
		t.Errorf("single-namespace JSON should have no separator:\n%s", out)
	}
}
