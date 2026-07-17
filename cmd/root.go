// Package cmd defines the command tree for the rossoctl CLI.
//
// Each command lives in its own file in this package and is attached to
// rootCmd in that file's init function. Commands stay thin: they parse and
// validate flags, then delegate to packages under internal/. This separation
// keeps the CLI surface (flags, help text, wiring) apart from the logic it
// drives, so the logic can be unit-tested without invoking Cobra.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/apiclient"
	"github.com/kagenti/rossoctl-cli/internal/config"
)

// These are set at build time via -ldflags. See the Makefile.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// defaultServer is the API endpoint used when --server is not supplied.
const defaultServer = "http://kagenti-ui.localtest.me:8080/api/v1/"

// Persistent flags shared by every command.
var (
	verbose bool
	server  string
)

// rootCmd is the base command invoked when the binary is run with no
// subcommand. It carries no RunE of its own; running it prints help.
var rootCmd = &cobra.Command{
	Use:   "rossoctl",
	Short: "rossoctl controls Rosso resources",
	Long: `rossoctl is a command-line interface for interacting with Rosso.

It follows the standard Go CLI layout: this cmd package wires up the Cobra
command tree, while the actual work is implemented in packages under internal/.`,
	// SilenceUsage/SilenceErrors let us render errors ourselves in Execute
	// rather than printing the full usage text on every runtime error.
	SilenceUsage:  true,
	SilenceErrors: true,
}

// serverOrDefault returns the explicitly supplied --server value, or the
// built-in default when --server was not given. Used as the seed value when a
// context must be created.
func serverOrDefault() string {
	if server != "" {
		return server
	}
	return defaultServer
}

// contextOverride is the name of a context to use instead of the current one.
// It is bound to the agents group's --context flag; empty means "use the
// current context".
var contextOverride string

// resolveContext returns the effective context: the one named by --context
// when that flag is set (error if no such context exists), otherwise the
// current context. The config is created (and seeded from the default server)
// on first use if it does not yet exist.
func resolveContext() (*config.Context, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	if contextOverride != "" {
		ctx, ok := cfg.Get(contextOverride)
		if !ok {
			return nil, fmt.Errorf("no context named %q", contextOverride)
		}
		return ctx, nil
	}
	cur, ok := cfg.Current()
	if !ok {
		// EnsureContext (via loadConfig) guarantees a current context; treat
		// its absence as a programming error rather than silently falling back.
		return nil, fmt.Errorf("no current context")
	}
	return cur, nil
}

// resolveServer determines the effective server URI and bearer token for a
// command:
//
//   - An explicit (non-empty) --server wins and overrides any context; no
//     token is used in that case.
//   - Otherwise the effective context (see resolveContext: --context, else the
//     current context) supplies both the server URI and its bearer token.
func resolveServer() (serverURI, token string, err error) {
	if server != "" {
		return server, "", nil
	}
	ctx, err := resolveContext()
	if err != nil {
		return "", "", err
	}
	return ctx.Server, ctx.BearerToken, nil
}

// currentNamespace returns the namespace of the effective context (see
// resolveContext). It returns an error if that context has no namespace set,
// since callers that need a namespace cannot proceed without one.
func currentNamespace() (string, error) {
	ctx, err := resolveContext()
	if err != nil {
		return "", err
	}
	if ctx.Namespace == "" {
		return "", fmt.Errorf("context %q has no namespace set; run `rossoctl config set-context --namespace <ns>`", ctx.Name)
	}
	return ctx.Namespace, nil
}

// newClient builds an API client for the effective server (see resolveServer).
// When --verbose is set, it attaches a logger that writes one line per HTTP
// request to the command's stderr, so verbose output never mixes with the
// --json/table results on stdout.
func newClient(cmd *cobra.Command) (*apiclient.Client, error) {
	serverURI, token, err := resolveServer()
	if err != nil {
		return nil, err
	}

	client := &apiclient.Client{BaseURL: serverURI, BearerToken: token}
	if verbose {
		errOut := cmd.ErrOrStderr()
		client.Logf = func(format string, args ...any) {
			fmt.Fprintf(errOut, format+"\n", args...)
		}
	}
	return client, nil
}

// Execute runs the root command. It is the single entry point called from
// main and is the only exported symbol in this package.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVar(&server, "server", "", "Rossoctl API server URI (overrides the current context; default: current context's server)")
}
