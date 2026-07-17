package cmd

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/deviceflow"
)

var loginToken string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in and store a bearer token on the current context",
	Long: `Obtain a bearer token and store it on the current context in
~/.rossoctl/config.yaml (created and seeded from the default server if absent).

With --token, the given token is stored directly. Without --token, an OAuth 2.0
device authorization flow is run against the server's Keycloak: rossoctl reads
the Keycloak URL, realm, and client id from GET <server>/auth/config, shows a
verification URL and one-time code (and attempts to open a browser), then polls
until you authorize.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		cur, ok := cfg.Current()
		if !ok {
			// loadConfig seeds a current context, so this should not happen.
			return fmt.Errorf("no current context to log in to")
		}

		token := loginToken
		if token == "" {
			token, err = deviceLogin(cmd)
			if err != nil {
				return err
			}
		}

		cur.BearerToken = token
		if err := cfg.Save(); err != nil {
			return err
		}

		cmd.Printf("Logged in; token set on context %q.\n", cur.Name)
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
