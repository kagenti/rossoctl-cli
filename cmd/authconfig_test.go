package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAuthConfigServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/config" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAuthConfigHumanReadable(t *testing.T) {
	srv := newAuthConfigServer(t, `{
		"enabled": true,
		"keycloak_url": "https://kc.example.com",
		"realm": "rossoctl",
		"client_id": "rossoctl-ui",
		"redirect_uri": null
	}`)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "auth-config")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"Authentication: enabled",
		"Keycloak URL",
		"https://kc.example.com",
		"rossoctl",
		"rossoctl-ui",
		"(not set)", // redirect_uri was null
	} {
		if !strings.Contains(out, want) {
			t.Errorf("auth-config output missing %q:\n%s", want, out)
		}
	}

	// The human output must not look like raw JSON.
	if strings.Contains(out, "\"enabled\"") {
		t.Errorf("human output unexpectedly contains raw JSON:\n%s", out)
	}
}

func TestAuthConfigJSON(t *testing.T) {
	srv := newAuthConfigServer(t, `{"enabled": false}`)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "auth-config", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	if decoded["enabled"] != false {
		t.Errorf("enabled = %v, want false", decoded["enabled"])
	}
}

func TestAuthConfigDisabled(t *testing.T) {
	srv := newAuthConfigServer(t, `{"enabled": false}`)

	out, err := execute(t, "--server", srv.URL+"/api/v1/", "auth-config")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Authentication: disabled") {
		t.Errorf("output = %q, want %q", out, "Authentication: disabled")
	}
}
