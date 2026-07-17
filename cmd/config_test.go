package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateHome points HOME at a fresh temp dir for this test so the context
// config starts empty and never touches the real home directory.
func isolateHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return filepath.Join(dir, ".rossoctl", "config.yaml")
}

func TestConfigGetContextsAutoCreates(t *testing.T) {
	path := isolateHome(t)

	// No --server: get-contexts should create and seed a context from the
	// built-in default server, and list it as current.
	out, err := execute(t, "config", "get-contexts")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file was not created: %v", err)
	}
	for _, want := range []string{"CURRENT", "NAME", "SERVER", defaultServer, "*"} {
		if !strings.Contains(out, want) {
			t.Errorf("get-contexts output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigCreateAndUseContext(t *testing.T) {
	isolateHome(t)

	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", "http://dev/api/v1/", "--bearer-token", "tok"); err != nil {
		t.Fatalf("create-context dev: %v", err)
	}
	// A second context should become current on creation.
	if _, err := execute(t, "config", "create-context",
		"--name", "prod", "--server", "http://prod/api/v1/"); err != nil {
		t.Fatalf("create-context prod: %v", err)
	}

	out, err := execute(t, "config", "get-contexts")
	if err != nil {
		t.Fatalf("get-contexts: %v", err)
	}
	// prod is current (most recently created); dev has a token.
	if !strings.Contains(out, "dev") || !strings.Contains(out, "<set>") {
		t.Errorf("expected dev with token <set>:\n%s", out)
	}
	// The current marker must be on the prod line.
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "prod") && !strings.Contains(line, "*") {
			t.Errorf("prod should be current:\n%s", out)
		}
		if strings.Contains(line, "dev") && strings.Contains(line, "*") {
			t.Errorf("dev should not be current:\n%s", out)
		}
	}

	// Switch to dev.
	if _, err := execute(t, "config", "use-context", "dev"); err != nil {
		t.Fatalf("use-context dev: %v", err)
	}
	out, _ = execute(t, "config", "get-contexts")
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "dev") && !strings.Contains(line, "*") {
			t.Errorf("dev should be current after use-context:\n%s", out)
		}
	}
}

func TestConfigUseContextUnknownErrors(t *testing.T) {
	isolateHome(t)
	if _, err := execute(t, "config", "use-context", "does-not-exist"); err == nil {
		t.Error("use-context on unknown name should error")
	}
}

func TestConfigCreateContextRequiresName(t *testing.T) {
	isolateHome(t)
	if _, err := execute(t, "config", "create-context", "--server", "http://x/"); err == nil {
		t.Error("create-context without --name should error")
	}
}

func TestConfigCreateContextNamespace(t *testing.T) {
	isolateHome(t)

	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", "http://dev/api/v1/", "--namespace", "team1"); err != nil {
		t.Fatalf("create-context: %v", err)
	}

	// The table shows a NAMESPACE column with the stored value.
	out, err := execute(t, "config", "get-contexts")
	if err != nil {
		t.Fatalf("get-contexts: %v", err)
	}
	if !strings.Contains(out, "NAMESPACE") {
		t.Errorf("get-contexts missing NAMESPACE column:\n%s", out)
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "dev") && !strings.Contains(line, "team1") {
			t.Errorf("dev context should show namespace team1:\n%s", out)
		}
	}

	// The namespace is persisted in the raw config.
	jsonOut, err := execute(t, "config", "get-contexts", "--json")
	if err != nil {
		t.Fatalf("get-contexts --json: %v", err)
	}
	if !strings.Contains(jsonOut, `"namespace": "team1"`) {
		t.Errorf("--json output missing namespace:\n%s", jsonOut)
	}
}

func TestConfigCreateContextNamespaceOmitted(t *testing.T) {
	isolateHome(t)

	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", "http://dev/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	// Omitted namespace renders as "-" and is omitted from stored JSON.
	out, _ := execute(t, "config", "get-contexts")
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "dev") && !strings.Contains(line, "-") {
			t.Errorf("dev context with no namespace should show '-':\n%s", out)
		}
	}
	jsonOut, _ := execute(t, "config", "get-contexts", "--json")
	if strings.Contains(jsonOut, `"namespace"`) {
		t.Errorf("empty namespace should be omitted from JSON:\n%s", jsonOut)
	}
}

// TestCurrentContextDrivesServer verifies that when --server is not given, the
// current context's server (and bearer token) are used, and that an explicit
// --server overrides the context (and drops the token).
func TestCurrentContextDrivesServer(t *testing.T) {
	isolateHome(t)

	var gotAuthByHost = map[string]string{}
	newSrv := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuthByHost[r.Host] = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/api/v1/namespaces" {
				_, _ = w.Write([]byte(`{"namespaces":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"items":[]}`))
		}))
	}
	ctxSrv := newSrv()
	defer ctxSrv.Close()
	overrideSrv := newSrv()
	defer overrideSrv.Close()

	// Point the current context at ctxSrv with a token.
	if _, err := execute(t, "config", "create-context",
		"--name", "ctx", "--server", ctxSrv.URL+"/api/v1/", "--bearer-token", "ctxtok"); err != nil {
		t.Fatalf("create-context: %v", err)
	}

	// No --server -> uses the context server + token.
	if _, err := execute(t, "agents", "list", "--namespaces", "team1"); err != nil {
		t.Fatalf("agents list via context: %v", err)
	}
	if got := gotAuthByHost[hostOf(t, ctxSrv.URL)]; got != "Bearer ctxtok" {
		t.Errorf("context request Authorization = %q, want %q", got, "Bearer ctxtok")
	}

	// Explicit --server overrides the context and drops the token.
	if _, err := execute(t, "--server", overrideSrv.URL+"/api/v1/",
		"agents", "list", "--namespaces", "team1"); err != nil {
		t.Fatalf("agents list via override: %v", err)
	}
	if got := gotAuthByHost[hostOf(t, overrideSrv.URL)]; got != "" {
		t.Errorf("override request should have no Authorization, got %q", got)
	}
}

func hostOf(t *testing.T, rawURL string) string {
	t.Helper()
	// httptest URLs are http://127.0.0.1:PORT; r.Host is the host:port.
	return strings.TrimPrefix(rawURL, "http://")
}
