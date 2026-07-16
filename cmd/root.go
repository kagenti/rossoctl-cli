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

// newClient builds an API client targeting the --server URI. When --verbose
// is set, it attaches a logger that writes one line per HTTP request to the
// command's stderr, so verbose output never mixes with the --json/table
// results on stdout.
func newClient(cmd *cobra.Command) *apiclient.Client {
	client := &apiclient.Client{BaseURL: server}
	if verbose {
		errOut := cmd.ErrOrStderr()
		client.Logf = func(format string, args ...any) {
			fmt.Fprintf(errOut, format+"\n", args...)
		}
	}
	return client
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
	rootCmd.PersistentFlags().StringVar(&server, "server", defaultServer, "Rossoctl API server URI")
}
