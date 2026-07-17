package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/apiclient"
)

var authConfigJSON bool

var authConfigCmd = &cobra.Command{
	Use:   "auth-config",
	Short: "Show the server's authentication configuration",
	Long: `Fetch and display the authentication configuration reported by the
server (GET <server>/auth/config).

By default the values are printed in a human-readable format. With --json the
raw JSON returned by the server is printed unchanged.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newClient(cmd)
		if err != nil {
			return err
		}
		cfg, err := client.GetAuthConfig(cmd.Context())
		if err != nil {
			return err
		}

		if authConfigJSON {
			return printAuthConfigJSON(cmd, cfg)
		}
		printAuthConfigHuman(cmd, cfg)
		return nil
	},
}

func printAuthConfigJSON(cmd *cobra.Command, cfg *apiclient.AuthConfig) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(cfg)
}

func printAuthConfigHuman(cmd *cobra.Command, cfg *apiclient.AuthConfig) {
	out := cmd.OutOrStdout()

	if !cfg.Enabled {
		fmt.Fprintln(out, "Authentication: disabled")
		return
	}

	fmt.Fprintln(out, "Authentication: enabled")

	rows := []struct {
		label string
		value *string
	}{
		{"Keycloak URL", cfg.KeycloakURL},
		{"Realm", cfg.Realm},
		{"Client ID", cfg.ClientID},
		{"Redirect URI", cfg.RedirectURI},
	}

	// Align the labels for readability.
	width := 0
	for _, r := range rows {
		if len(r.label) > width {
			width = len(r.label)
		}
	}

	for _, r := range rows {
		value := "(not set)"
		if r.value != nil && strings.TrimSpace(*r.value) != "" {
			value = *r.value
		}
		fmt.Fprintf(out, "  %-*s  %s\n", width, r.label+":", value)
	}
}

func init() {
	authConfigCmd.Flags().BoolVar(&authConfigJSON, "json", false, "print the raw JSON response unchanged")
	rootCmd.AddCommand(authConfigCmd)
}
