package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

var uiOpenCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the rossoctl UI",
	Long: `Open the rossoctl UI in your default browser.

The URL is derived from the current context's server: its scheme, hostname, and
port are used, and any path is dropped (so the UI is opened at the site root).
It fails if the current context has no server set.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, err := resolveContext()
		if err != nil {
			return err
		}
		if ctx.Server == "" {
			return fmt.Errorf("context %q has no server set", ctx.Name)
		}

		u, err := url.Parse(ctx.Server)
		if err != nil {
			return fmt.Errorf("context %q has an invalid server %q: %w", ctx.Name, ctx.Server, err)
		}
		if u.Host == "" {
			return fmt.Errorf("context %q has an invalid server %q: no host", ctx.Name, ctx.Server)
		}

		// Keep only the scheme and host (host includes the port); drop the path,
		// query, and fragment so the UI opens at the site root.
		openURL := (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()

		if err := browserOpener(openURL); err != nil {
			return fmt.Errorf("failed to open %s in a browser: %w", openURL, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Opened %s in your browser\n", openURL)
		return nil
	},
}

func init() {
	uiCmd := newGroup("ui", "Interact with the rossoctl UI")
	uiCmd.AddCommand(uiOpenCmd)
	rootCmd.AddCommand(uiCmd)
}
