package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// statusBodies holds the JSON each of the three status endpoints returns.
type statusBodies struct {
	authStatus string
	me         string
	platform   string
}

func newStatusServer(t *testing.T, b statusBodies) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/status":
			_, _ = w.Write([]byte(b.authStatus))
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(b.me))
		case "/api/v1/config/platform-status":
			_, _ = w.Write([]byte(b.platform))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// authenticatedBodies is a fully-populated set: auth enabled, an authenticated
// user with roles, and a platform status with components and registry info.
var authenticatedBodies = statusBodies{
	authStatus: `{"enabled": true, "authenticated": true, "keycloak_url": "https://kc.example.com", "realm": "rossoctl", "client_id": "rossoctl-ui"}`,
	me:         `{"username": "alice", "email": "alice@example.com", "roles": ["admin", "viewer"], "authenticated": true}`,
	platform: `{
		"components": [
			{"name": "Istio", "status": "Ready"},
			{"name": "Keycloak", "status": "Degraded"},
			{"name": "SPIRE", "status": "Missing"}
		],
		"registry": {
			"clusterBuildStrategyPresent": true,
			"clusterBuildStrategies": ["buildah", "kaniko"],
			"registryEndpoint": "registry.example.com:5000"
		}
	}`,
}

func TestStatusText(t *testing.T) {
	srv := newStatusServer(t, authenticatedBodies)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	for _, want := range []string{
		"Current Session",
		"Authentication:", "Enabled",
		"Status:", "Authenticated",
		"Username:", "alice",
		"Email:", "alice@example.com",
		"Roles:", "admin, viewer",
		"Platform Status",
		"Istio:", "Ready",
		"Keycloak:", "Degraded",
		"SPIRE:", "Missing",
		"Registry & Build",
		"ClusterBuildStrategy:", "Present",
		"Strategies:", "buildah, kaniko",
		"Registry endpoint:", "registry.example.com:5000",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}

	// It must not be raw JSON.
	if strings.Contains(out, "\"components\"") {
		t.Errorf("human output unexpectedly contains raw JSON:\n%s", out)
	}
}

func TestStatusJSON(t *testing.T) {
	srv := newStatusServer(t, authenticatedBodies)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "status", "--json")
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}

	var decoded struct {
		Session struct {
			Enabled       bool `json:"enabled"`
			Authenticated bool `json:"authenticated"`
		} `json:"session"`
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		Platform struct {
			Components []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"components"`
			Registry struct {
				RegistryEndpoint string `json:"registryEndpoint"`
			} `json:"registry"`
		} `json:"platform"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	if !decoded.Session.Enabled || !decoded.Session.Authenticated {
		t.Errorf("session not decoded as enabled+authenticated: %+v", decoded.Session)
	}
	if decoded.User.Username != "alice" {
		t.Errorf("user.username = %q, want alice", decoded.User.Username)
	}
	if len(decoded.Platform.Components) != 3 {
		t.Errorf("components = %d, want 3", len(decoded.Platform.Components))
	}
	if decoded.Platform.Registry.RegistryEndpoint != "registry.example.com:5000" {
		t.Errorf("registryEndpoint = %q", decoded.Platform.Registry.RegistryEndpoint)
	}
}

func TestStatusGuestOmitsUserDetails(t *testing.T) {
	// Auth disabled, guest user: the UI shows Disabled/Guest and no user block.
	srv := newStatusServer(t, statusBodies{
		authStatus: `{"enabled": false, "authenticated": false}`,
		me:         `{"username": "guest", "email": null, "roles": [], "authenticated": false}`,
		platform: `{
			"components": [],
			"registry": {"clusterBuildStrategyPresent": false, "clusterBuildStrategies": [], "registryEndpoint": ""}
		}`,
	})

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	for _, want := range []string{
		"Authentication:", "Disabled",
		"Status:", "Guest",
		"No components reported.",
		"ClusterBuildStrategy:", "Missing",
		"Registry endpoint:", "—",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("guest status output missing %q:\n%s", want, out)
		}
	}
	// The guest user's details must not be shown.
	if strings.Contains(out, "Username:") {
		t.Errorf("guest output should omit Username:\n%s", out)
	}
}
