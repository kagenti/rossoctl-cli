package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newNamespacesServer(t *testing.T, body string) (*httptest.Server, *string) {
	t.Helper()
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotQuery
}

func TestNamespacesListTable(t *testing.T) {
	srv, gotQuery := newNamespacesServer(t, `{"namespaces": ["team2", "team1", "default"]}`)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "namespaces", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default (enabled-only) sends no query parameter.
	if *gotQuery != "" {
		t.Errorf("query = %q, want empty (enabled-only default)", *gotQuery)
	}

	if !strings.Contains(out, "NAME") {
		t.Errorf("table missing header:\n%s", out)
	}
	for _, ns := range []string{"default", "team1", "team2"} {
		if !strings.Contains(out, ns) {
			t.Errorf("table missing %q:\n%s", ns, out)
		}
	}
	// Sorted output: default before team1 before team2.
	if idx := strings.Index(out, "default"); idx > strings.Index(out, "team1") {
		t.Errorf("namespaces not sorted:\n%s", out)
	}
	// Human output must not be raw JSON.
	if strings.Contains(out, "\"namespaces\"") {
		t.Errorf("human output unexpectedly contains raw JSON:\n%s", out)
	}
}

func TestNamespacesListJSON(t *testing.T) {
	srv, _ := newNamespacesServer(t, `{"namespaces": ["a", "b"]}`)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "namespaces", "list", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded struct {
		Namespaces []string `json:"namespaces"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	if len(decoded.Namespaces) != 2 {
		t.Errorf("expected 2 namespaces, got %d", len(decoded.Namespaces))
	}
}

func TestNamespacesListAllFlag(t *testing.T) {
	srv, gotQuery := newNamespacesServer(t, `{"namespaces": ["default"]}`)

	if _, err := execute(t, "--server", srv.URL+"/api/v1/", "namespaces", "list", "--all"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *gotQuery != "enabled_only=false" {
		t.Errorf("query = %q, want %q", *gotQuery, "enabled_only=false")
	}
}

func TestNamespacesListEmpty(t *testing.T) {
	srv, _ := newNamespacesServer(t, `{"namespaces": []}`)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "namespaces", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No namespaces found") {
		t.Errorf("empty output = %q, want %q", out, "No namespaces found")
	}
}
