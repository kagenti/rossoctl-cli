package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kagenti/rossoctl-cli/internal/config"
)

func TestLoginSetsTokenOnCurrentContext(t *testing.T) {
	path := isolateHome(t)

	// Establish a known current context.
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", "http://dev/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}

	out, err := execute(t, "login", "--token", "sekret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(out, "dev") {
		t.Errorf("login output should name the context:\n%s", out)
	}

	// The token must be persisted on the current context.
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cur, ok := cfg.Current()
	if !ok {
		t.Fatal("no current context after login")
	}
	if cur.Name != "dev" {
		t.Errorf("current context = %q, want dev", cur.Name)
	}
	if cur.BearerToken != "sekret" {
		t.Errorf("token = %q, want sekret", cur.BearerToken)
	}
}

// deviceLoginServer serves both the rossoctl /auth/config endpoint and the
// Keycloak device+token endpoints. The token endpoint returns
// authorization_pending once, then the access token, exercising the poll loop.
func deviceLoginServer(t *testing.T, enabled bool) *httptest.Server {
	t.Helper()
	var tokenCalls int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/auth/config":
			if !enabled {
				_, _ = w.Write([]byte(`{"enabled": false}`))
				return
			}
			// keycloak_url points back at this same server.
			_, _ = w.Write([]byte(`{"enabled":true,"keycloak_url":"` + srv.URL +
				`","realm":"rossoctl","client_id":"rossoctl-ui","redirect_uri":null}`))
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/auth/device"):
			_, _ = w.Write([]byte(`{"device_code":"DEV","user_code":"WDJB-MJHT",` +
				`"verification_uri":"` + srv.URL + `/device","expires_in":600,"interval":1}`))
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			tokenCalls++
			if tokenCalls < 2 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"DEVICE-TOKEN"}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLoginDeviceFlow(t *testing.T) {
	path := isolateHome(t)
	srv := deviceLoginServer(t, true)

	// Point the current context at the mock server (also serves Keycloak).
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}

	// No --token: runs the device flow and saves the resulting token.
	if _, err := execute(t, "login"); err != nil {
		t.Fatalf("login (device flow): %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cur, _ := cfg.Current()
	if cur.BearerToken != "DEVICE-TOKEN" {
		t.Errorf("token = %q, want DEVICE-TOKEN", cur.BearerToken)
	}
}

func TestLoginDeviceFlowPrintsCode(t *testing.T) {
	isolateHome(t)
	srv := deviceLoginServer(t, true)
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}

	// The verification URL and user code are shown on stderr; stdout stays for
	// the final confirmation only.
	_, stderr, err := executeSplit(t, "login")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(stderr, "WDJB-MJHT") {
		t.Errorf("stderr missing user code:\n%s", stderr)
	}
	if !strings.Contains(stderr, "/device") {
		t.Errorf("stderr missing verification URL:\n%s", stderr)
	}
}

func TestLoginDeviceFlowAuthDisabled(t *testing.T) {
	isolateHome(t)
	srv := deviceLoginServer(t, false) // enabled=false
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}

	if _, err := execute(t, "login"); err == nil {
		t.Error("login should error when server auth is disabled and no --token given")
	}
}

func TestLoginSeedsContextWhenMissing(t *testing.T) {
	path := isolateHome(t)

	// No context yet: login should seed one (from the default server) and set
	// the token on it.
	if _, err := execute(t, "login", "--token", "tok"); err != nil {
		t.Fatalf("login: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cur, ok := cfg.Current()
	if !ok {
		t.Fatal("login did not create a current context")
	}
	if cur.Server != defaultServer {
		t.Errorf("seeded server = %q, want %q", cur.Server, defaultServer)
	}
	if cur.BearerToken != "tok" {
		t.Errorf("token = %q, want tok", cur.BearerToken)
	}
}
