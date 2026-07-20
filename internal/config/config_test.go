package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func tmpConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".rossoctl", "config.yaml")
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	cfg, err := Load(tmpConfigPath(t))
	if err != nil {
		t.Fatalf("Load of missing file should not error: %v", err)
	}
	if len(cfg.Contexts) != 0 || cfg.CurrentContext != "" {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestSaveCreatesDirAndFileWithPerms(t *testing.T) {
	path := tmpConfigPath(t)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Upsert(Context{Name: "a", Server: "http://a/"})
	if err := cfg.SetCurrent("a"); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perm = %o, want 700", perm)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perm = %o, want 600", perm)
	}
}

func TestSaveChmodsExistingFile(t *testing.T) {
	path := tmpConfigPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-create the file with loose perms.
	if err := os.WriteFile(path, []byte("contexts: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Upsert(Context{Name: "a", Server: "http://a/"})
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fileInfo, _ := os.Stat(path)
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perm after Save = %o, want 600", perm)
	}
}

func TestRoundTrip(t *testing.T) {
	path := tmpConfigPath(t)
	cfg, _ := Load(path)
	cfg.Upsert(Context{Name: "dev", Server: "http://dev/", Namespace: "team1", BearerToken: "tok"})
	cfg.Upsert(Context{Name: "prod", Server: "http://prod/"})
	_ = cfg.SetCurrent("prod")
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.CurrentContext != "prod" {
		t.Errorf("current = %q, want prod", reloaded.CurrentContext)
	}
	if len(reloaded.Contexts) != 2 {
		t.Fatalf("got %d contexts, want 2", len(reloaded.Contexts))
	}
	dev, ok := reloaded.Get("dev")
	if !ok || dev.Server != "http://dev/" || dev.Namespace != "team1" || dev.BearerToken != "tok" {
		t.Errorf("dev context round-trip mismatch: %+v (ok=%v)", dev, ok)
	}
}

func TestUpsertReplacesByName(t *testing.T) {
	cfg := &Config{}
	cfg.Upsert(Context{Name: "a", Server: "http://old/"})
	cfg.Upsert(Context{Name: "a", Server: "http://new/", BearerToken: "t"})
	if len(cfg.Contexts) != 1 {
		t.Fatalf("expected 1 context after replace, got %d", len(cfg.Contexts))
	}
	if cfg.Contexts[0].Server != "http://new/" || cfg.Contexts[0].BearerToken != "t" {
		t.Errorf("upsert did not replace: %+v", cfg.Contexts[0])
	}
}

func TestSetCurrentUnknownErrors(t *testing.T) {
	cfg := &Config{}
	if err := cfg.SetCurrent("nope"); err == nil {
		t.Error("SetCurrent on unknown name should error")
	}
}

func TestRename(t *testing.T) {
	cfg := &Config{}
	cfg.Upsert(Context{Name: "old", Server: "http://s/", Namespace: "team1"})
	_ = cfg.SetCurrent("old")

	if err := cfg.Rename("old", "new"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, ok := cfg.Get("old"); ok {
		t.Error("old name should be gone after rename")
	}
	ctx, ok := cfg.Get("new")
	if !ok {
		t.Fatal("new name not found after rename")
	}
	// Other fields are preserved.
	if ctx.Server != "http://s/" || ctx.Namespace != "team1" {
		t.Errorf("rename lost fields: %+v", ctx)
	}
	// Renaming the current context updates the reference.
	if cfg.CurrentContext != "new" {
		t.Errorf("CurrentContext = %q, want new", cfg.CurrentContext)
	}
}

func TestRenameNonCurrentLeavesCurrent(t *testing.T) {
	cfg := &Config{}
	cfg.Upsert(Context{Name: "a", Server: "http://a/"})
	cfg.Upsert(Context{Name: "b", Server: "http://b/"})
	_ = cfg.SetCurrent("a")

	if err := cfg.Rename("b", "b2"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if cfg.CurrentContext != "a" {
		t.Errorf("CurrentContext = %q, want a (unchanged)", cfg.CurrentContext)
	}
}

func TestRenameErrors(t *testing.T) {
	cfg := &Config{}
	cfg.Upsert(Context{Name: "a", Server: "http://a/"})
	cfg.Upsert(Context{Name: "b", Server: "http://b/"})

	if err := cfg.Rename("missing", "x"); err == nil {
		t.Error("renaming an unknown context should error")
	}
	if err := cfg.Rename("a", "b"); err == nil {
		t.Error("renaming to an existing name should error")
	}
	if err := cfg.Rename("a", ""); err == nil {
		t.Error("renaming to an empty name should error")
	}
	// No-op rename is allowed.
	if err := cfg.Rename("a", "a"); err != nil {
		t.Errorf("no-op rename should be allowed, got %v", err)
	}
}

func TestCurrent(t *testing.T) {
	cfg := &Config{}
	if _, ok := cfg.Current(); ok {
		t.Error("empty config should have no current context")
	}
	cfg.Upsert(Context{Name: "a", Server: "http://a/"})
	_ = cfg.SetCurrent("a")
	cur, ok := cfg.Current()
	if !ok || cur.Name != "a" {
		t.Errorf("Current() = %+v, ok=%v; want context a", cur, ok)
	}
}

func TestEnsureContextSeedsFromDefault(t *testing.T) {
	path := tmpConfigPath(t)
	cfg, err := EnsureContext(path, "http://seed-host:8080/api/v1/", nil)
	if err != nil {
		t.Fatalf("EnsureContext: %v", err)
	}
	cur, ok := cfg.Current()
	if !ok {
		t.Fatal("expected a current context after seeding")
	}
	// The context is named after the server's hostname (no port/path), but its
	// server field keeps the full URI.
	if cur.Name != "seed-host" {
		t.Errorf("seeded context name = %q, want seed-host", cur.Name)
	}
	if cur.Server != "http://seed-host:8080/api/v1/" {
		t.Errorf("seeded server = %q, want the full URI", cur.Server)
	}
	if cur.BearerToken != "" {
		t.Errorf("seeded token = %q, want empty", cur.BearerToken)
	}

	// The seed must have been persisted.
	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Contexts) != 1 {
		t.Errorf("expected 1 persisted context, got %d", len(reloaded.Contexts))
	}
}

func TestContextNameForServer(t *testing.T) {
	cases := []struct{ server, want string }{
		{"http://kagenti-ui.localtest.me:8080/api/v1/", "kagenti-ui.localtest.me"},
		{"https://api.example.com/api/v1/", "api.example.com"},
		{"http://127.0.0.1:9090/", "127.0.0.1"},
		{"not a url", "not a url"},         // unparseable -> raw fallback
		{"/relative/path", "/relative/path"}, // no host -> raw fallback
	}
	for _, c := range cases {
		if got := ContextNameForServer(c.server); got != c.want {
			t.Errorf("ContextNameForServer(%q) = %q, want %q", c.server, got, c.want)
		}
	}
}

func TestEnsureContextLeavesExistingAlone(t *testing.T) {
	path := tmpConfigPath(t)
	first, _ := Load(path)
	first.Upsert(Context{Name: "mine", Server: "http://mine/"})
	_ = first.SetCurrent("mine")
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}

	cfg, err := EnsureContext(path, "http://seed/", nil)
	if err != nil {
		t.Fatalf("EnsureContext: %v", err)
	}
	if len(cfg.Contexts) != 1 || cfg.CurrentContext != "mine" {
		t.Errorf("EnsureContext altered existing config: %+v", cfg)
	}
}

func TestEnsureContextSeedsNamespaceFromResolver(t *testing.T) {
	path := tmpConfigPath(t)
	firstNS := func(server, token string) (string, error) {
		if token != "" {
			t.Errorf("seed resolver got token %q, want empty (seed is unauthenticated)", token)
		}
		if server != "http://seed-host/" {
			t.Errorf("seed resolver got server %q", server)
		}
		return "team1", nil
	}
	cfg, err := EnsureContext(path, "http://seed-host/", firstNS)
	if err != nil {
		t.Fatalf("EnsureContext: %v", err)
	}
	cur, ok := cfg.Current()
	if !ok {
		t.Fatal("expected a current context after seeding")
	}
	if cur.Namespace != "team1" {
		t.Errorf("seeded namespace = %q, want team1", cur.Namespace)
	}
}

func TestEnsureContextResolverErrorAbortsSeed(t *testing.T) {
	path := tmpConfigPath(t)
	wantErr := errors.New("boom")
	firstNS := func(server, token string) (string, error) {
		return "", wantErr
	}
	_, err := EnsureContext(path, "http://seed-host/", firstNS)
	if !errors.Is(err, wantErr) {
		t.Fatalf("EnsureContext error = %v, want %v", err, wantErr)
	}
	// A failed seed must not write a config file.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("expected no config file after aborted seed, stat err = %v", statErr)
	}
}
