package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetAuthConfig(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"enabled": true,
			"keycloak_url": "https://kc.example.com",
			"realm": "rossoctl",
			"client_id": "rossoctl-ui",
			"redirect_uri": null
		}`))
	}))
	defer srv.Close()

	// Base URL includes an /api/v1/ prefix, like the real default, to ensure
	// the endpoint path is appended rather than replacing the prefix.
	c := &Client{BaseURL: srv.URL + "/api/v1/"}
	cfg, err := c.GetAuthConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/api/v1/auth/config" {
		t.Errorf("requested path = %q, want %q", gotPath, "/api/v1/auth/config")
	}
	if !cfg.Enabled {
		t.Error("Enabled = false, want true")
	}
	if cfg.KeycloakURL == nil || *cfg.KeycloakURL != "https://kc.example.com" {
		t.Errorf("KeycloakURL = %v, want https://kc.example.com", cfg.KeycloakURL)
	}
	if cfg.RedirectURI != nil {
		t.Errorf("RedirectURI = %v, want nil", cfg.RedirectURI)
	}
}

func TestGetAuthConfigBaseWithoutTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/config" {
			t.Errorf("requested path = %q, want %q", r.URL.Path, "/api/v1/auth/config")
		}
		_, _ = w.Write([]byte(`{"enabled": false}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/api/v1"} // no trailing slash
	if _, err := c.GetAuthConfig(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListTools(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"items":[
			{"name":"weather-mcp","namespace":"team1","description":"d","status":"Ready",
			 "labels":{"protocol":["mcp"],"framework":null,"type":"tool"},
			 "workloadType":"deployment","createdAt":null}
		]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/api/v1/"}
	resp, err := c.ListTools(context.Background(), "team1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/api/v1/tools" {
		t.Errorf("path = %q, want %q", gotPath, "/api/v1/tools")
	}
	if gotQuery != "namespace=team1" {
		t.Errorf("query = %q, want %q", gotQuery, "namespace=team1")
	}
	if len(resp.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(resp.Items))
	}
	tl := resp.Items[0]
	if tl.Name != "weather-mcp" || tl.Status != "Ready" {
		t.Errorf("unexpected tool: %+v", tl)
	}
	if len(tl.Labels.Protocol) != 1 || tl.Labels.Protocol[0] != "mcp" {
		t.Errorf("protocol = %v, want [mcp]", tl.Labels.Protocol)
	}
}

func TestListAgents(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"items":[
			{"name":"a","namespace":"team1","description":"d","status":"Ready",
			 "labels":{"protocol":["a2a"],"framework":"LangGraph","type":"agent"},
			 "workloadType":"deployment","createdAt":"2026-01-01T00:00:00Z"}
		]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/api/v1/"}
	resp, err := c.ListAgents(context.Background(), "team1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/api/v1/agents" {
		t.Errorf("path = %q, want %q", gotPath, "/api/v1/agents")
	}
	if gotQuery != "namespace=team1" {
		t.Errorf("query = %q, want %q", gotQuery, "namespace=team1")
	}
	if len(resp.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(resp.Items))
	}
	a := resp.Items[0]
	if a.Name != "a" || a.Status != "Ready" {
		t.Errorf("unexpected agent: %+v", a)
	}
	if len(a.Labels.Protocol) != 1 || a.Labels.Protocol[0] != "a2a" {
		t.Errorf("protocol = %v, want [a2a]", a.Labels.Protocol)
	}
}

func TestListAgentsNoNamespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query string, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/api/v1/"}
	if _, err := c.ListAgents(context.Background(), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListNamespaces(t *testing.T) {
	tests := []struct {
		name        string
		enabledOnly bool
		wantQuery   string
	}{
		{name: "enabled only (default)", enabledOnly: true, wantQuery: ""},
		{name: "all", enabledOnly: false, wantQuery: "enabled_only=false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath, gotQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotQuery = r.URL.RawQuery
				_, _ = w.Write([]byte(`{"namespaces":["default","team1"]}`))
			}))
			defer srv.Close()

			c := &Client{BaseURL: srv.URL + "/api/v1/"}
			resp, err := c.ListNamespaces(context.Background(), tt.enabledOnly)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotPath != "/api/v1/namespaces" {
				t.Errorf("path = %q, want %q", gotPath, "/api/v1/namespaces")
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
			if len(resp.Namespaces) != 2 {
				t.Errorf("got %d namespaces, want 2", len(resp.Namespaces))
			}
		})
	}
}

func TestLogfCalledPerRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"enabled": false}`))
	}))
	defer srv.Close()

	var logs []string
	c := &Client{
		BaseURL: srv.URL + "/api/v1/",
		Logf:    func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) },
	}
	if _, err := c.GetAuthConfig(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(logs) != 2 {
		t.Fatalf("expected 2 log lines (request + response), got %d: %v", len(logs), logs)
	}
	if !strings.HasPrefix(logs[0], "GET "+srv.URL+"/api/v1/auth/config") {
		t.Errorf("first log = %q, want a GET request line", logs[0])
	}
	if !strings.Contains(logs[1], "200 OK") {
		t.Errorf("second log = %q, want response status", logs[1])
	}
}

func TestNilLogfIsSafe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"enabled": false}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/api/v1/"} // Logf nil
	if _, err := c.GetAuthConfig(context.Background()); err != nil {
		t.Fatalf("unexpected error with nil Logf: %v", err)
	}
}

func TestGetAuthConfigServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/api/v1/"}
	if _, err := c.GetAuthConfig(context.Background()); err == nil {
		t.Fatal("expected an error on 500, got nil")
	}
}
