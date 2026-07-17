package cmd

import (
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

func TestLoginRequiresToken(t *testing.T) {
	isolateHome(t)
	if _, err := execute(t, "login"); err == nil {
		t.Error("login without --token should error")
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
