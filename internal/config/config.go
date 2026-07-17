// Package config manages rossoctl's persisted context configuration, stored
// in ~/.rossoctl/config.yaml (kubectl-style).
//
// A Config holds a list of named contexts — each with a server URI and an
// optional bearer token — plus the name of the current context. Like the
// other internal packages it is free of Cobra: callers pass an explicit file
// path so it can be unit-tested against a temporary directory.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// dirPerm and filePerm are the required permissions for the config
	// directory and file: private to the owner (the file may hold a token).
	dirPerm  os.FileMode = 0o700
	filePerm os.FileMode = 0o600
)

// Context is a single named target: a server URI, an optional default
// namespace, and an optional bearer token.
type Context struct {
	Name        string `yaml:"name" json:"name"`
	Server      string `yaml:"server" json:"server"`
	Namespace   string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	BearerToken string `yaml:"bearerToken,omitempty" json:"bearerToken,omitempty"`
}

// Config is the on-disk configuration: the set of contexts and which one is
// current.
type Config struct {
	CurrentContext string    `yaml:"currentContext" json:"currentContext"`
	Contexts       []Context `yaml:"contexts" json:"contexts"`

	// path is where this Config was loaded from / will be saved to. It is not
	// serialized.
	path string `yaml:"-" json:"-"`
}

// DefaultPath returns ~/.rossoctl/config.yaml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".rossoctl", "config.yaml"), nil
}

// Load reads and parses the config at path. A missing file is not an error:
// an empty Config (with path remembered) is returned so the caller can seed
// and Save it. A present but unreadable or malformed file is an error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{path: path}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	cfg := &Config{path: path}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	cfg.path = path // Unmarshal doesn't touch the unexported field, but be explicit.
	return cfg, nil
}

// Save writes the config back to its path, creating the parent directory
// (0700) and the file (0600) if needed. Existing files are chmod'd to 0600 so
// they conform even if they predated this code.
func (c *Config) Save() error {
	if c.path == "" {
		return fmt.Errorf("config has no path to save to")
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	// Ensure the directory has the required perms even if it already existed.
	if err := os.Chmod(dir, dirPerm); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", dir, err)
	}

	if err := os.WriteFile(c.path, data, filePerm); err != nil {
		return fmt.Errorf("writing %s: %w", c.path, err)
	}
	// WriteFile only applies perms when creating; chmod an existing file too.
	if err := os.Chmod(c.path, filePerm); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", c.path, err)
	}
	return nil
}

// Path returns the file this config is bound to.
func (c *Config) Path() string { return c.path }

// Get returns the named context, or false if there is none.
func (c *Config) Get(name string) (*Context, bool) {
	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			return &c.Contexts[i], true
		}
	}
	return nil, false
}

// Current returns the current context, or false if none is set or it is
// missing from the list.
func (c *Config) Current() (*Context, bool) {
	if c.CurrentContext == "" {
		return nil, false
	}
	return c.Get(c.CurrentContext)
}

// Upsert adds ctx, replacing any existing context with the same name.
func (c *Config) Upsert(ctx Context) {
	for i := range c.Contexts {
		if c.Contexts[i].Name == ctx.Name {
			c.Contexts[i] = ctx
			return
		}
	}
	c.Contexts = append(c.Contexts, ctx)
}

// SetCurrent makes name the current context. It errors if name is unknown.
func (c *Config) SetCurrent(name string) error {
	if _, ok := c.Get(name); !ok {
		return fmt.Errorf("no context named %q", name)
	}
	c.CurrentContext = name
	return nil
}

// EnsureContext loads the config at path and, if it contains no contexts,
// seeds one from defaultServer (using the server URI as the context name, with
// an empty bearer token), makes it current, and saves. The resulting config is
// returned. This is the single place the lazy create-if-missing behavior
// lives.
func EnsureContext(path, defaultServer string) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	if len(cfg.Contexts) == 0 {
		cfg.Upsert(Context{Name: defaultServer, Server: defaultServer})
		if err := cfg.SetCurrent(defaultServer); err != nil {
			return nil, err
		}
		if err := cfg.Save(); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}
