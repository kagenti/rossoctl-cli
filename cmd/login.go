package cmd

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/config"
	"github.com/kagenti/rossoctl-cli/internal/deviceflow"
)

var loginToken string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in and store a bearer token on a context",
	Long: `Obtain a bearer token and store it on a context in ~/.rossoctl/config.yaml.

With --server, the token is stored on the context named after that server's
hostname, creating it if none exists, and that context becomes current.
Without --server, the token is stored on the current context (a context is
created from the default server if the config is empty).

With --token, the given token is stored directly. Without --token, an OAuth 2.0
device authorization flow is run against the server's Keycloak: rossoctl reads
the Keycloak URL, realm, and client id from GET <server>/auth/config, shows a
verification URL and one-time code (and attempts to open a browser), then polls
until you authorize.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		// Load without the lazy default-server seed: login sets the token and
		// then fetches the namespace itself (with that token), so it must not
		// let an unauthenticated seed attempt the namespaces fetch first.
		cfg, err := loadConfigNoSeed()
		if err != nil {
			return err
		}

		// Authenticate first, so that when a context is created we can fetch its
		// default namespace with the freshly-obtained token (the namespaces
		// endpoint may require authorization).
		token := loginToken
		if token == "" {
			token, err = deviceLogin(cmd)
			if err != nil {
				return err
			}
		}

		// Determine which context to log into:
		//   - With an explicit --server, target the context named after that
		//     server's hostname, creating it if none exists, and make it
		//     current.
		//   - Without --server, log into the current context, seeding one from
		//     the default server if the config is empty.
		var target *config.Context
		var created bool // whether login created the target context this run
		if cmd.Flags().Changed("server") {
			name := config.ContextNameForServer(server)
			if existing, ok := cfg.Get(name); ok {
				target = existing
			} else {
				cfg.Upsert(config.Context{Name: name, Server: server})
				target, _ = cfg.Get(name)
				created = true
			}
			if err := cfg.SetCurrent(name); err != nil {
				return err
			}
		} else {
			cur, ok := cfg.Current()
			if !ok {
				// Empty config: seed a context from the default server (named
				// after its hostname) and make it current, mirroring the lazy
				// seed but without a namespace fetch (login does that below).
				name := config.ContextNameForServer(serverOrDefault())
				cfg.Upsert(config.Context{Name: name, Server: serverOrDefault()})
				if err := cfg.SetCurrent(name); err != nil {
					return err
				}
				cur, _ = cfg.Get(name)
				created = true
			}
			target = cur
		}

		target.BearerToken = token

		// Default the namespace to the server's first namespace when the target
		// context doesn't have one yet, authorizing with the token just
		// obtained. A context that already has a namespace is left untouched.
		//
		// The fetch only hard-fails when login just created the context (there
		// is no prior namespace to fall back to). For a re-login onto an
		// existing namespace-less context, the fetch is best-effort: setting the
		// token is login's core job and must not be blocked by a namespaces
		// outage.
		if target.Namespace == "" {
			ns, err := firstNamespaceFor(cmd.Context(), target.Server, token, verboseLogf(cmd))
			if err != nil {
				if created {
					return err
				}
				fmt.Fprintf(cmd.ErrOrStderr(),
					"Warning: could not default the namespace for context %q: %v\n", target.Name, err)
			} else {
				target.Namespace = ns
			}
		}

		if err := cfg.Save(); err != nil {
			return err
		}

		cmd.Printf("Logged in; token set on context %q.\n", target.Name)
		return nil
	},
}

// deviceLogin runs the OAuth device authorization flow against the server's
// Keycloak and returns the resulting access token.
func deviceLogin(cmd *cobra.Command) (string, error) {
	// Read Keycloak details from the resolved server's auth config.
	client, err := newClient(cmd)
	if err != nil {
		return "", err
	}
	authCfg, err := client.GetAuthConfig(cmd.Context())
	if err != nil {
		return "", err
	}
	if !authCfg.Enabled {
		return "", fmt.Errorf("authentication is not enabled on the server; use --token instead")
	}
	kcURL := deref(authCfg.KeycloakURL)
	realm := deref(authCfg.Realm)
	clientID := deref(authCfg.ClientID)
	if kcURL == "-" || realm == "-" || clientID == "-" {
		return "", fmt.Errorf("server auth config is missing keycloak_url, realm, or client_id")
	}

	df := &deviceflow.Client{
		KeycloakURL: kcURL,
		Realm:       realm,
		ClientID:    clientID,
		Sleep:       deviceflowSleep,
	}
	if verbose {
		errOut := cmd.ErrOrStderr()
		df.Logf = func(format string, args ...any) {
			fmt.Fprintf(errOut, format+"\n", args...)
		}
	}

	da, err := df.RequestDeviceCode(cmd.Context())
	if err != nil {
		return "", err
	}

	// Prompt on stderr so stdout stays clean, then best-effort open a browser.
	errOut := cmd.ErrOrStderr()
	fmt.Fprintf(errOut, "To sign in, visit:\n  %s\nand enter code: %s\n",
		da.VerificationURI, da.UserCode)

	openURL := da.VerificationURIComplete
	if openURL == "" {
		openURL = da.VerificationURI
	}
	if err := browserOpener(openURL); err == nil {
		fmt.Fprintf(errOut, "(opened %s in your browser)\n", openURL)
	}
	fmt.Fprintln(errOut, "Waiting for authorization...")

	return df.PollToken(cmd.Context(), da)
}

// browserOpener opens a URL in the user's browser. It is a variable so tests
// can replace it with a no-op, avoiding spawning a real browser process.
var browserOpener = openBrowser

// deviceflowSleep is the wait between device-flow token polls. It is a
// variable (nil => the deviceflow package uses time.Sleep) so tests can
// replace it with a no-op to avoid real delays.
var deviceflowSleep func(d time.Duration)

// openBrowser attempts to open url in the user's default browser. It is
// best-effort: any error (including an unsupported platform) is returned for
// the caller to ignore.
func openBrowser(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default: // linux, bsd, etc.
		name = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(name, args...).Start()
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "bearer token to store on the current context; if omitted, run the device login flow")
	rootCmd.AddCommand(loginCmd)
}
