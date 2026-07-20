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

	// Establish a known current context. It is given a namespace so that
	// neither create-context nor login needs to fetch namespaces from the
	// (unreachable) server.
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", "http://dev/api/v1/", "--namespace", "team1"); err != nil {
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

func TestLoginReloginNamespacelessContextStillSetsToken(t *testing.T) {
	path := isolateHome(t)

	// An existing, namespace-less context pointed at an unreachable server.
	seedNamespacelessContext(t, path, "dev", "http://unreachable.invalid/api/v1/")

	// Re-login must still set the token even though the namespace fetch fails:
	// setting the token is login's core job. The failure is only a warning.
	_, stderr, err := executeSplit(t, "login", "--token", "sekret")
	if err != nil {
		t.Fatalf("login should not fail when the namespace fetch fails on an existing context: %v", err)
	}
	if !strings.Contains(stderr, "Warning") {
		t.Errorf("expected a warning about the namespace default, got:\n%s", stderr)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cur, _ := cfg.Current()
	if cur.BearerToken != "sekret" {
		t.Errorf("token = %q, want sekret (must be set despite the namespace fetch failure)", cur.BearerToken)
	}
	if cur.Namespace != "" {
		t.Errorf("namespace = %q, want empty (fetch failed, best-effort leaves it blank)", cur.Namespace)
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

	// Point the current context at the mock server (also serves Keycloak). A
	// namespace is set so neither create-context nor login fetches namespaces.
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/", "--namespace", "team1"); err != nil {
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
		"--name", "dev", "--server", srv.URL+"/api/v1/", "--namespace", "team1"); err != nil {
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
		"--name", "dev", "--server", srv.URL+"/api/v1/", "--namespace", "team1"); err != nil {
		t.Fatalf("create-context: %v", err)
	}

	if _, err := execute(t, "login"); err == nil {
		t.Error("login should error when server auth is disabled and no --token given")
	}
}

func TestLoginServerDefaultsNamespaceUsingToken(t *testing.T) {
	path := isolateHome(t)

	// Capture the Authorization header the namespaces call carries: login must
	// authenticate first, then fetch namespaces with the obtained token.
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"namespaces":["team2","team1"]}`))
	}))
	t.Cleanup(srv.Close)

	server := srv.URL + "/api/v1/"
	if _, err := execute(t, "login", "--server", server, "--token", "sekret"); err != nil {
		t.Fatalf("login --server: %v", err)
	}

	if gotAuth != "Bearer sekret" {
		t.Errorf("namespaces request Authorization = %q, want %q (token must authorize the fetch)", gotAuth, "Bearer sekret")
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cur, _ := cfg.Current()
	if cur.Namespace != "team1" {
		t.Errorf("defaulted namespace = %q, want team1 (first after sorting)", cur.Namespace)
	}
	if cur.BearerToken != "sekret" {
		t.Errorf("token = %q, want sekret", cur.BearerToken)
	}
}

func TestLoginServerCreatesHostnameContext(t *testing.T) {
	path := isolateHome(t)

	// A pre-existing current context, unrelated to the --server host.
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", "http://dev/api/v1/", "--namespace", "team1"); err != nil {
		t.Fatalf("create-context: %v", err)
	}

	// The --server host must be reachable because login now defaults the new
	// context's namespace from GET <server>/namespaces.
	srv := namespacesServer(t, "team1")
	newServer := srv.URL + "/api/v1/"
	newName := config.ContextNameForServer(newServer)

	// login --server for a NEW host must create a context named after that
	// host, set the token there, and make it current.
	if _, err := execute(t, "login", "--server", newServer, "--token", "tok"); err != nil {
		t.Fatalf("login --server: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ctx, ok := cfg.Get(newName)
	if !ok {
		t.Fatalf("expected a context named after the server hostname %q", newName)
	}
	if ctx.Server != newServer {
		t.Errorf("server = %q, want the full URI %q", ctx.Server, newServer)
	}
	if ctx.BearerToken != "tok" {
		t.Errorf("token = %q, want tok", ctx.BearerToken)
	}
	if cfg.CurrentContext != newName {
		t.Errorf("current context = %q, want %q", cfg.CurrentContext, newName)
	}
	// The pre-existing dev context must be untouched.
	if dev, ok := cfg.Get("dev"); !ok || dev.BearerToken != "" {
		t.Errorf("dev context should be unchanged, got %+v (ok=%v)", dev, ok)
	}
}

func TestLoginServerReusesExistingHostnameContext(t *testing.T) {
	path := isolateHome(t)

	// A context already exists for the host (its name IS the hostname).
	if _, err := execute(t, "config", "create-context",
		"--name", "newhost", "--server", "http://newhost:8080/api/v1/", "--namespace", "team1"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	// Switch away so it isn't current (namespace set so create-context does
	// not fetch from the unreachable server).
	if _, err := execute(t, "config", "create-context",
		"--name", "other", "--server", "http://other/api/v1/", "--namespace", "team1"); err != nil {
		t.Fatalf("create-context other: %v", err)
	}

	if _, err := execute(t, "login", "--server", "http://newhost:8080/api/v1/", "--token", "tok"); err != nil {
		t.Fatalf("login --server: %v", err)
	}

	cfg, _ := config.Load(path)
	// No duplicate context was created.
	count := 0
	for _, c := range cfg.Contexts {
		if c.Name == "newhost" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one 'newhost' context, got %d", count)
	}
	// The existing context got the token, kept its namespace, and is current.
	ctx, _ := cfg.Get("newhost")
	if ctx.BearerToken != "tok" {
		t.Errorf("token = %q, want tok", ctx.BearerToken)
	}
	if ctx.Namespace != "team1" {
		t.Errorf("namespace = %q, want team1 (preserved)", ctx.Namespace)
	}
	if cfg.CurrentContext != "newhost" {
		t.Errorf("current context = %q, want newhost", cfg.CurrentContext)
	}
}

func TestLoginNoServerUsesCurrentContext(t *testing.T) {
	path := isolateHome(t)

	// Both contexts get a namespace so neither create-context nor login
	// fetches namespaces from the unreachable servers.
	if _, err := execute(t, "config", "create-context",
		"--name", "a", "--server", "http://a/api/v1/", "--namespace", "team1"); err != nil {
		t.Fatalf("create-context a: %v", err)
	}
	// b is current.
	if _, err := execute(t, "config", "create-context",
		"--name", "b", "--server", "http://b/api/v1/", "--namespace", "team1"); err != nil {
		t.Fatalf("create-context b: %v", err)
	}

	before, _ := config.Load(path)
	countBefore := len(before.Contexts)

	// No --server: token goes on the current context (b), no new context.
	if _, err := execute(t, "login", "--token", "tok"); err != nil {
		t.Fatalf("login: %v", err)
	}

	cfg, _ := config.Load(path)
	if len(cfg.Contexts) != countBefore {
		t.Errorf("login without --server should not add a context: had %d, now %d", countBefore, len(cfg.Contexts))
	}
	b, _ := cfg.Get("b")
	if b.BearerToken != "tok" {
		t.Errorf("current context b token = %q, want tok", b.BearerToken)
	}
	if a, _ := cfg.Get("a"); a.BearerToken != "" {
		t.Errorf("non-current context a should be untouched, got token %q", a.BearerToken)
	}
}
