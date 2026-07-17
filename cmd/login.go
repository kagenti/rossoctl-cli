package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var loginToken string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Set the bearer token on the current context",
	Long: `Store a bearer token on the current context.

The token from --token is written to the current context in
~/.rossoctl/config.yaml (which is created and seeded from the default server if
it does not yet exist).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if loginToken == "" {
			return fmt.Errorf("--token is required")
		}

		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		cur, ok := cfg.Current()
		if !ok {
			// loadConfig seeds a current context, so this should not happen.
			return fmt.Errorf("no current context to log in to")
		}

		cur.BearerToken = loginToken
		if err := cfg.Save(); err != nil {
			return err
		}

		cmd.Printf("Logged in; token set on context %q.\n", cur.Name)
		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "bearer token to store on the current context (required)")
	rootCmd.AddCommand(loginCmd)
}
