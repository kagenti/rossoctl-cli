package cortexclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rossoctl/rossoctl-cli/internal/apiclient"
)

func TestNewFileClientDefaults(t *testing.T) {
	// With XDG unset, XDGConfig falls back to $HOME/.config and ConfigDir to
	// XDGConfig/rossoctl. HOME is pointed at a temp dir so the files
	// NewFileClient seeds land there, not in the developer's real home.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", "")

	wantXDG := filepath.Join(home, ".config")
	wantConfigDir := filepath.Join(wantXDG, "rossoctl")

	c := NewFileClient("prod")
	if c.Name != "prod" {
		t.Errorf("Name = %q, want %q", c.Name, "prod")
	}
	if c.XDGConfig != wantXDG {
		t.Errorf("XDGConfig = %q, want %q", c.XDGConfig, wantXDG)
	}
	if c.ConfigDir != wantConfigDir {
		t.Errorf("ConfigDir = %q, want %q", c.ConfigDir, wantConfigDir)
	}
	if want := filepath.Join(wantConfigDir, "prod", "agents.json"); c.AgentsFilename != want {
		t.Errorf("AgentsFilename = %q, want %q", c.AgentsFilename, want)
	}
	if want := filepath.Join(wantConfigDir, "prod", "rossocortex-state.json"); c.StateFilename != want {
		t.Errorf("StateFilename = %q, want %q", c.StateFilename, want)
	}
}

func TestNewFileClientXDGOverride(t *testing.T) {
	// XDG_CONFIG_HOME set, ROSSOCORTEX_CONFIG_DIR unset: ConfigDir derives from
	// the XDG value.
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", "")

	c := NewFileClient("dev")
	if c.XDGConfig != xdg {
		t.Errorf("XDGConfig = %q, want %q", c.XDGConfig, xdg)
	}
	wantConfigDir := filepath.Join(xdg, "rossoctl")
	if c.ConfigDir != wantConfigDir {
		t.Errorf("ConfigDir = %q, want %q", c.ConfigDir, wantConfigDir)
	}
	if want := filepath.Join(wantConfigDir, "dev", "agents.json"); c.AgentsFilename != want {
		t.Errorf("AgentsFilename = %q, want %q", c.AgentsFilename, want)
	}
}

func TestNewFileClientConfigDirOverride(t *testing.T) {
	// ROSSOCORTEX_CONFIG_DIR wins over the XDG-derived default.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "/custom/xdg")
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", dir)

	c := NewFileClient("ctx")
	if c.ConfigDir != dir {
		t.Errorf("ConfigDir = %q, want %q", c.ConfigDir, dir)
	}
	if want := filepath.Join(dir, "ctx", "agents.json"); c.AgentsFilename != want {
		t.Errorf("AgentsFilename = %q, want %q", c.AgentsFilename, want)
	}
	if want := filepath.Join(dir, "ctx", "rossocortex-state.json"); c.StateFilename != want {
		t.Errorf("StateFilename = %q, want %q", c.StateFilename, want)
	}
}

// TestNewFileClientSeedsFiles verifies NewFileClient creates the agents and
// state files with their empty-seed contents when they don't already exist.
func TestNewFileClientSeedsFiles(t *testing.T) {
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", t.TempDir())

	c := NewFileClient("ctx")

	agents, err := os.ReadFile(c.AgentsFilename)
	if err != nil {
		t.Fatalf("agents file not created: %v", err)
	}
	if got := string(agents); got != `{"agents":{}}`+"\n" {
		t.Errorf("agents seed = %q, want empty agents map", got)
	}
	state, err := os.ReadFile(c.StateFilename)
	if err != nil {
		t.Fatalf("state file not created: %v", err)
	}
	if got := string(state); got != "{}\n" {
		t.Errorf("state seed = %q, want empty object", got)
	}
}

// TestNewFileClientDoesNotClobber verifies existing files are left untouched.
func TestNewFileClientDoesNotClobber(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", dir)

	agentsPath := filepath.Join(dir, "ctx", "agents.json")
	if err := os.MkdirAll(filepath.Dir(agentsPath), 0o700); err != nil {
		t.Fatal(err)
	}
	existing := `{"agents":{"keep":{"token":"t","created":"2024-01-01","ports":{},"budget":1}}}`
	if err := os.WriteFile(agentsPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	NewFileClient("ctx")

	got, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != existing {
		t.Errorf("existing agents file was modified:\n got %q\nwant %q", got, existing)
	}
}

func TestListAgents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", dir)

	c := NewFileClient("ctx")
	// Overwrite the seeded file with two agents.
	content := `{"agents":{
		"beta":  {"token":"tb","created":"2024-02-02T00:00:00Z","ports":{"forward":1,"reverse":2,"transparent":3,"stats":4,"session":5},"budget":2.5},
		"alpha": {"token":"ta","created":"2024-01-01T00:00:00Z","ports":{"forward":10,"reverse":20,"transparent":30,"stats":40,"session":50},"budget":1.0}
	}}`
	if err := os.WriteFile(c.AgentsFilename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	resp, err := c.ListAgents(t.Context(), "")
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(resp.Items))
	}
	// Sorted by name: alpha before beta.
	if resp.Items[0].Name != "alpha" || resp.Items[1].Name != "beta" {
		t.Errorf("names = %q, %q; want alpha, beta", resp.Items[0].Name, resp.Items[1].Name)
	}
	// CreatedAt comes from the agent's `created` field.
	if resp.Items[0].CreatedAt == nil || *resp.Items[0].CreatedAt != "2024-01-01T00:00:00Z" {
		t.Errorf("alpha CreatedAt = %v, want 2024-01-01T00:00:00Z", resp.Items[0].CreatedAt)
	}
	if resp.Items[1].CreatedAt == nil || *resp.Items[1].CreatedAt != "2024-02-02T00:00:00Z" {
		t.Errorf("beta CreatedAt = %v, want 2024-02-02T00:00:00Z", resp.Items[1].CreatedAt)
	}
}

func TestListAgentsEmpty(t *testing.T) {
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", t.TempDir())
	c := NewFileClient("ctx") // seeds an empty agents map

	resp, err := c.ListAgents(t.Context(), "")
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("got %d items, want 0", len(resp.Items))
	}
}

func TestListAgentsMalformed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", dir)
	c := NewFileClient("ctx")
	if err := os.WriteFile(c.AgentsFilename, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListAgents(t.Context(), ""); err == nil {
		t.Error("ListAgents should error on a malformed agents file")
	}
}

func TestCreateAgent(t *testing.T) {
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", t.TempDir())

	// Fix the clock so Created is deterministic.
	fixed := time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)
	orig := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = orig })

	c := NewFileClient("ctx")

	resp, err := c.CreateAgent(t.Context(), &apiclient.CreateAgentRequest{Name: "svc", Namespace: "ns"})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if !resp.Success || resp.Name != "svc" || resp.Namespace != "ns" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if resp.Message != "Agent metadata set, agent creation is not implemented" {
		t.Errorf("message = %q", resp.Message)
	}

	// The entry is persisted with the current time as Created.
	agents, err := c.loadAgents()
	if err != nil {
		t.Fatalf("loadAgents: %v", err)
	}
	got, ok := agents["svc"]
	if !ok {
		t.Fatalf("agent %q not written to %s", "svc", c.AgentsFilename)
	}
	if want := fixed.Format(time.RFC3339); got.Created != want {
		t.Errorf("Created = %q, want %q", got.Created, want)
	}

	// It also shows up via ListAgents.
	list, err := c.ListAgents(t.Context(), "")
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "svc" {
		t.Errorf("ListAgents = %+v, want one item named svc", list.Items)
	}
	if list.Items[0].CreatedAt == nil || *list.Items[0].CreatedAt != fixed.Format(time.RFC3339) {
		t.Errorf("CreatedAt = %v", list.Items[0].CreatedAt)
	}
}

func TestCreateAgentPreservesOthers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", dir)

	c := NewFileClient("ctx")
	// Pre-populate an existing agent with full fields.
	seed := `{"agents":{"keep":{"token":"tok","created":"2020-01-01T00:00:00Z","ports":{"forward":1,"reverse":2,"transparent":3,"stats":4,"session":5},"budget":9.5}}}`
	if err := os.WriteFile(c.AgentsFilename, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := c.CreateAgent(t.Context(), &apiclient.CreateAgentRequest{Name: "new"}); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	agents, err := c.loadAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 2 {
		t.Fatalf("got %d agents, want 2 (keep + new)", len(agents))
	}
	keep := agents["keep"]
	if keep.Token != "tok" || keep.Budget != 9.5 || keep.Ports.Session != 5 || keep.Created != "2020-01-01T00:00:00Z" {
		t.Errorf("existing agent was altered: %+v", keep)
	}
}

func TestFileClientLogfHook(t *testing.T) {
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", t.TempDir())

	var got []string
	c := NewFileClient("ctx")
	c.Logf = func(format string, args ...any) {
		got = append(got, format)
	}
	if _, err := c.ListNamespaces(t.Context(), true); err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	if len(got) == 0 {
		t.Error("expected Logf to be called for ListNamespaces")
	}

	// A nil Logf must not panic.
	c2 := NewFileClient("ctx")
	if _, err := c2.ListNamespaces(t.Context(), true); err != nil {
		t.Fatalf("ListNamespaces (nil Logf): %v", err)
	}
}
