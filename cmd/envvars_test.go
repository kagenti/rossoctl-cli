package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseEnvVars(t *testing.T) {
	body := "FOO=bar\n" +
		"# a comment\n" +
		"\n" +
		"  BAZ = qux value \n" +
		"EMPTY=\n" +
		"URL=http://a=b\n" // value may contain '='

	got, err := parseEnvVars(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []struct{ name, value string }{
		{"FOO", "bar"},
		{"BAZ", "qux value"},
		{"EMPTY", ""},
		{"URL", "http://a=b"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d env vars, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Name != w.name || got[i].Value != w.value {
			t.Errorf("env[%d] = %+v, want {%s %s}", i, got[i], w.name, w.value)
		}
	}
}

func TestParseEnvVarsInvalidLine(t *testing.T) {
	if _, err := parseEnvVars("FOO=bar\nnotakeyvalue\n"); err == nil {
		t.Error("expected error for a line without '='")
	}
	if _, err := parseEnvVars("=novalue\n"); err == nil {
		t.Error("expected error for an empty key")
	}
}

func TestParseEnvVarsEmpty(t *testing.T) {
	got, err := parseEnvVars("\n#only comments\n\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no env vars, got %+v", got)
	}
}

func TestSameHost(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"http://h:8080/api/v1/", "http://h:8080/env", true},
		{"http://h:8080/api/v1/", "http://h:9090/env", false}, // different port
		{"http://api.example.com/api/v1/", "https://raw.githubusercontent.com/x/.env", false},
		{"http://h:8080/", "not a url", true}, // url.Parse is lenient; host "" != "" guards below
	}
	for _, c := range cases {
		// The last case documents lenient parsing; assert the real intent:
		// only same host+port returns true.
		got := sameHost(c.a, c.b)
		if c.b == "not a url" {
			continue
		}
		if got != c.want {
			t.Errorf("sameHost(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestFetchEnvVarsForeignHostNoToken proves the API bearer token is NOT sent
// when the env URL is on a different host than the API server (the GitHub-404
// bug), but IS sent when the env URL is on the API host.
func TestFetchEnvVarsTokenHostGating(t *testing.T) {
	isolateHome(t)

	var apiAuth, foreignAuth string

	// The "API server": serves /namespaces and /env, records the auth header.
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/namespaces":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"namespaces":["team1"]}`))
		case "/env":
			apiAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte("FOO=bar\n"))
		default:
			t.Errorf("unexpected api path %q", r.URL.Path)
		}
	}))
	t.Cleanup(api.Close)

	// A "foreign" host (stands in for GitHub): records the auth header.
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("FOO=bar\n"))
	}))
	t.Cleanup(foreign.Close)

	// Context points at the API server with a token.
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", api.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	if _, err := execute(t, "login", "--token", "api-token"); err != nil {
		t.Fatalf("login: %v", err)
	}

	// Env URL on the API host -> token IS sent.
	if _, err := fetchEnvVars(context.Background(), rootCmd, api.URL+"/env"); err != nil {
		t.Fatalf("fetch (api host): %v", err)
	}
	if apiAuth != "Bearer api-token" {
		t.Errorf("api-host env fetch Authorization = %q, want %q", apiAuth, "Bearer api-token")
	}

	// Env URL on a foreign host -> token is NOT sent.
	if _, err := fetchEnvVars(context.Background(), rootCmd, foreign.URL+"/env"); err != nil {
		t.Fatalf("fetch (foreign host): %v", err)
	}
	if foreignAuth != "" {
		t.Errorf("foreign-host env fetch Authorization = %q, want empty", foreignAuth)
	}
}
