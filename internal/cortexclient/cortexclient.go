// Package cortexclient provides FileClient, a file-backed implementation of
// the rossoctlclient.Rossoctl interface.
//
// Where apiclient.Client talks to a live Rossoctl API server over HTTP,
// FileClient serves the same operations from local state (a "cortex"
// context). It reuses the apiclient request/response types so callers can hold
// a rossoctlclient.Rossoctl without caring which backend is behind it.
package cortexclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rossoctl/rossoctl-cli/internal/apiclient"
)

// now returns the current time. It is a variable so tests can substitute a
// fixed clock when asserting on the Created timestamp.
var now = time.Now

// Ports is the set of TCP port numbers an agent listens on.
type Ports struct {
	Forward     int `json:"forward"`
	Reverse     int `json:"reverse"`
	Transparent int `json:"transparent"`
	Stats       int `json:"stats"`
	Session     int `json:"session"`
}

// Agent is one agent's persisted state in the agents file. The agent's name is
// the key it is stored under (see agentsFile), not a field here.
type Agent struct {
	Token   string  `json:"token"`
	Created string  `json:"created"` // timestamp serialized as a string
	Ports   Ports   `json:"ports"`
	Budget  float64 `json:"budget"`
}

// agentsFile is the on-disk shape of AgentsFilename: an "agents" key mapping
// each agent name to its Agent record.
type agentsFile struct {
	Agents map[string]Agent `json:"agents"`
}

// FileClient is a file-backed Rossoctl backend. Its on-disk locations are
// derived from the context Name and the environment (see NewFileClient).
type FileClient struct {
	// Name is the context this client reads and writes its state from.
	Name string

	// Logf, if set, is called to log each operation. The command layer wires
	// this to stderr when --verbose is given; when nil, no logging happens.
	// Kept as a plain function so this package stays free of any logging or CLI
	// dependency.
	Logf func(format string, args ...any)

	// XDGConfig is the base config directory: $XDG_CONFIG_HOME, or ~/.config
	// when that is unset.
	XDGConfig string

	// ConfigDir is where this client's state lives: $ROSSOCORTEX_CONFIG_DIR, or
	// XDGConfig/rossoctl when that is unset.
	ConfigDir string

	// AgentsFilename is the per-context agents file: ConfigDir/<name>/agents.json.
	AgentsFilename string

	// StateFilename is the per-context state file:
	// ConfigDir/<name>/rossocortex-state.json.
	StateFilename string
}

// NewFileClient returns a FileClient for the named context, resolving its
// config directory and file locations from the environment:
//
//   - XDGConfig is $XDG_CONFIG_HOME, defaulting to ~/.config.
//   - ConfigDir is $ROSSOCORTEX_CONFIG_DIR, defaulting to XDGConfig/rossoctl.
//   - AgentsFilename is ConfigDir/<name>/agents.json.
//   - StateFilename is ConfigDir/<name>/rossocortex-state.json.
//
// It creates AgentsFilename and StateFilename on disk if they do not already
// exist (seeding an empty agents map and empty state, respectively). Existing
// files are left untouched, and any I/O error while seeding is ignored so
// construction never fails — the file operations report errors when used.
func NewFileClient(name string) *FileClient {
	home, _ := os.UserHomeDir()
	xdgConfig := envOr("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	configDir := envOr("ROSSOCORTEX_CONFIG_DIR", filepath.Join(xdgConfig, "rossoctl"))

	c := &FileClient{
		Name:           name,
		XDGConfig:      xdgConfig,
		ConfigDir:      configDir,
		AgentsFilename: filepath.Join(configDir, name, "agents.json"),
		StateFilename:  filepath.Join(configDir, name, "rossocortex-state.json"),
	}

	seedFileIfMissing(c.AgentsFilename, []byte(`{"agents":{}}`+"\n"))
	seedFileIfMissing(c.StateFilename, []byte("{}\n"))

	return c
}

// seedFileIfMissing creates path (and its parent directory) with the given
// contents when it does not yet exist. Existing files are left untouched. Any
// error is ignored: seeding is best-effort so NewFileClient cannot fail.
func seedFileIfMissing(path string, contents []byte) {
	if _, err := os.Stat(path); err == nil {
		return // already exists
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	// O_EXCL so a file that appears between the Stat and here is not clobbered.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(contents)
}

// envOr returns the value of environment variable key, or def when it is unset
// or empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// logf logs through Logf when it is set; otherwise it is a no-op.
func (c *FileClient) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

// errNotImplemented is returned by operations that FileClient does not yet
// support. The file-backed backend is a scaffold; the methods exist to satisfy
// rossoctlclient.Rossoctl and will be filled in as the cortex format lands.
func errNotImplemented(op string) error {
	return fmt.Errorf("FileClient: %s not implemented", op)
}

// GetAuthConfig implements rossoctlclient.Rossoctl.
func (c *FileClient) GetAuthConfig(ctx context.Context) (*apiclient.AuthConfig, error) {
	return nil, errNotImplemented("GetAuthConfig")
}

// ListAgents implements rossoctlclient.Rossoctl. It reads AgentsFilename and
// returns one AgentSummary per stored agent, with Name taken from the map key
// and CreatedAt from the agent's Created timestamp. The namespace argument is
// ignored: the file-backed backend is not namespaced.
func (c *FileClient) ListAgents(ctx context.Context, namespace string) (*apiclient.AgentListResponse, error) {
	c.logf("cortex %q: ListAgents (agents %s)", c.Name, c.AgentsFilename)

	agents, err := c.loadAgents()
	if err != nil {
		return nil, err
	}

	// Sort by name so the output is deterministic (map order is random).
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)

	resp := &apiclient.AgentListResponse{Items: make([]apiclient.AgentSummary, 0, len(names))}
	for _, name := range names {
		created := agents[name].Created
		resp.Items = append(resp.Items, apiclient.AgentSummary{
			Name:      name,
			CreatedAt: &created,
		})
	}
	return resp, nil
}

// loadAgents reads and decodes the "agents" map from AgentsFilename. A missing
// file yields an empty map (not an error); a present but malformed file is an
// error.
func (c *FileClient) loadAgents() (map[string]Agent, error) {
	data, err := os.ReadFile(c.AgentsFilename)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Agent{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", c.AgentsFilename, err)
	}
	var file agentsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", c.AgentsFilename, err)
	}
	if file.Agents == nil {
		return map[string]Agent{}, nil
	}
	return file.Agents, nil
}

// GetAgent implements rossoctlclient.Rossoctl.
func (c *FileClient) GetAgent(ctx context.Context, namespace, name string) (*apiclient.AgentDetail, error) {
	return nil, errNotImplemented("GetAgent")
}

// DeleteAgent implements rossoctlclient.Rossoctl.
func (c *FileClient) DeleteAgent(ctx context.Context, namespace, name string) (*apiclient.DeleteResponse, error) {
	return nil, errNotImplemented("DeleteAgent")
}

// CreateAgent implements rossoctlclient.Rossoctl. It records an Agent entry for
// req.Name in AgentsFilename with Created set to the current time, then reports
// that only the metadata was written. Actually launching the agent is not yet
// implemented.
func (c *FileClient) CreateAgent(ctx context.Context, req *apiclient.CreateAgentRequest) (*apiclient.CreateAgentResponse, error) {
	c.logf("cortex %q: CreateAgent %q (agents %s)", c.Name, req.Name, c.AgentsFilename)

	agents, err := c.loadAgents()
	if err != nil {
		return nil, err
	}

	entry := agents[req.Name]
	entry.Created = now().UTC().Format(time.RFC3339)
	agents[req.Name] = entry

	if err := c.saveAgents(agents); err != nil {
		return nil, err
	}

	// The command layer prints Message; the agent metadata is recorded but
	// launching the agent itself is not yet implemented.
	return &apiclient.CreateAgentResponse{
		Success:   true,
		Name:      req.Name,
		Namespace: req.Namespace,
		Message:   "Agent metadata set, agent creation is not implemented",
	}, nil
}

// saveAgents writes agents back to AgentsFilename under the "agents" key,
// creating the parent directory if needed.
func (c *FileClient) saveAgents(agents map[string]Agent) error {
	data, err := json.MarshalIndent(agentsFile{Agents: agents}, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding agents: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(c.AgentsFilename), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(c.AgentsFilename), err)
	}
	if err := os.WriteFile(c.AgentsFilename, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", c.AgentsFilename, err)
	}
	return nil
}

// ListTools implements rossoctlclient.Rossoctl.
func (c *FileClient) ListTools(ctx context.Context, namespace string) (*apiclient.ToolListResponse, error) {
	return nil, errNotImplemented("ListTools")
}

// DeleteTool implements rossoctlclient.Rossoctl.
func (c *FileClient) DeleteTool(ctx context.Context, namespace, name string) (*apiclient.DeleteResponse, error) {
	return nil, errNotImplemented("DeleteTool")
}

// CreateTool implements rossoctlclient.Rossoctl.
func (c *FileClient) CreateTool(ctx context.Context, req *apiclient.CreateToolRequest) (*apiclient.CreateToolResponse, error) {
	return nil, errNotImplemented("CreateTool")
}

// ListNamespaces implements rossoctlclient.Rossoctl.
func (c *FileClient) ListNamespaces(ctx context.Context, enabledOnly bool) (*apiclient.NamespaceListResponse, error) {
	c.logf("cortex %q: ListNamespaces (state %s)", c.Name, c.StateFilename)
	return &apiclient.NamespaceListResponse{
		Namespaces: []string{"default"},
	}, nil
}
