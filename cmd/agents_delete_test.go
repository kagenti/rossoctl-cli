package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentsDelete(t *testing.T) {
	isolateHome(t)

	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces" {
			_, _ = w.Write([]byte(`{"namespaces":["team1"]}`))
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"success": true, "message": "Deployment 'orders' deleted; Service 'orders' deleted"}`))
	}))
	t.Cleanup(srv.Close)

	// Current context: namespace team1, server srv.
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	if _, err := execute(t, "config", "set-context", "--namespace", "team1"); err != nil {
		t.Fatalf("set-context: %v", err)
	}

	out, err := execute(t, "agents", "delete", "orders")
	if err != nil {
		t.Fatalf("agents delete: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/agents/team1/orders" {
		t.Errorf("path = %q, want /api/v1/agents/team1/orders", gotPath)
	}
	// The server's message is surfaced to the user.
	if !strings.Contains(out, "Deployment 'orders' deleted") {
		t.Errorf("output missing server message:\n%s", out)
	}
}

func TestAgentsDeleteNamespaceOverride(t *testing.T) {
	isolateHome(t)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces" {
			_, _ = w.Write([]byte(`{"namespaces":["team1","team2"]}`))
			return
		}
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"success": true, "message": "deleted"}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	if _, err := execute(t, "config", "set-context", "--namespace", "team1"); err != nil {
		t.Fatalf("set-context: %v", err)
	}

	// --namespace team2 overrides the context's team1.
	if _, err := execute(t, "agents", "--namespace", "team2", "delete", "orders"); err != nil {
		t.Fatalf("agents delete: %v", err)
	}
	if gotPath != "/api/v1/agents/team2/orders" {
		t.Errorf("path = %q, want /api/v1/agents/team2/orders", gotPath)
	}
}

func TestAgentsDeleteRequiresNamespace(t *testing.T) {
	path := isolateHome(t)
	// Context without a namespace (seeded directly so create-context's
	// namespace auto-default does not run).
	seedNamespacelessContext(t, path, "dev", "http://x/api/v1/")
	if _, err := execute(t, "agents", "delete", "orders"); err == nil {
		t.Error("agents delete should error when the current context has no namespace")
	}
}

func TestAgentsDeleteServerError(t *testing.T) {
	isolateHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces" {
			_, _ = w.Write([]byte(`{"namespaces":["team1"]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Agent 'ghost' not found in namespace 'team1'"}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	if _, err := execute(t, "config", "set-context", "--namespace", "team1"); err != nil {
		t.Fatalf("set-context: %v", err)
	}

	if _, err := execute(t, "agents", "delete", "ghost"); err == nil {
		t.Error("agents delete should return an error on a 404 response")
	}
}
