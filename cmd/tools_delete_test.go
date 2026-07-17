package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToolsDelete(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"success": true, "message": "Deployment 'weather-mcp' deleted; Service 'weather-mcp' deleted"}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	if _, err := execute(t, "config", "set-context", "--namespace", "team1"); err != nil {
		t.Fatalf("set-context: %v", err)
	}

	out, err := execute(t, "tools", "delete", "weather-mcp")
	if err != nil {
		t.Fatalf("tools delete: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/tools/team1/weather-mcp" {
		t.Errorf("path = %q, want /api/v1/tools/team1/weather-mcp", gotPath)
	}
	if !strings.Contains(out, "Deployment 'weather-mcp' deleted") {
		t.Errorf("output missing server message:\n%s", out)
	}
}

func TestToolsDeleteNamespaceOverride(t *testing.T) {
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

	if _, err := execute(t, "tools", "--namespace", "team2", "delete", "weather-mcp"); err != nil {
		t.Fatalf("tools delete: %v", err)
	}
	if gotPath != "/api/v1/tools/team2/weather-mcp" {
		t.Errorf("path = %q, want /api/v1/tools/team2/weather-mcp", gotPath)
	}
}

func TestToolsDeleteServerError(t *testing.T) {
	isolateHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces" {
			_, _ = w.Write([]byte(`{"namespaces":["team1"]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Tool 'ghost' not found"}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	if _, err := execute(t, "config", "set-context", "--namespace", "team1"); err != nil {
		t.Fatalf("set-context: %v", err)
	}

	if _, err := execute(t, "tools", "delete", "ghost"); err == nil {
		t.Error("tools delete should return an error on a 404 response")
	}
}

func TestToolsContextOverride(t *testing.T) {
	isolateHome(t)

	// Two servers: current context vs the --context target.
	var currentHit, otherPath string
	current := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces" {
			_, _ = w.Write([]byte(`{"namespaces":["team1"]}`))
			return
		}
		currentHit = r.URL.Path
		_, _ = w.Write([]byte(`{"success": true, "message": "deleted"}`))
	}))
	t.Cleanup(current.Close)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces" {
			_, _ = w.Write([]byte(`{"namespaces":["team9"]}`))
			return
		}
		otherPath = r.URL.Path
		_, _ = w.Write([]byte(`{"success": true, "message": "deleted"}`))
	}))
	t.Cleanup(other.Close)

	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", current.URL+"/api/v1/", "--namespace", "team1"); err != nil {
		t.Fatalf("create-context dev: %v", err)
	}
	if _, err := execute(t, "config", "create-context",
		"--name", "prod", "--server", other.URL+"/api/v1/", "--namespace", "team9"); err != nil {
		t.Fatalf("create-context prod: %v", err)
	}
	if _, err := execute(t, "config", "use-context", "dev"); err != nil {
		t.Fatalf("use-context dev: %v", err)
	}

	// --context prod routes to the prod server + namespace.
	if _, err := execute(t, "tools", "--context", "prod", "delete", "weather-mcp"); err != nil {
		t.Fatalf("tools delete --context prod: %v", err)
	}
	if otherPath != "/api/v1/tools/team9/weather-mcp" {
		t.Errorf("prod path = %q, want /api/v1/tools/team9/weather-mcp", otherPath)
	}
	if currentHit != "" {
		t.Errorf("current (dev) server should not have been called, got %q", currentHit)
	}
}
