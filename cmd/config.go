package cmd

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/config"
)

// loadConfig returns the context config, creating and seeding it from the
// effective default server if it does not yet exist. This is the lazy
// create-if-missing behavior: the file is only touched when a context command
// needs to inspect it.
func loadConfig() (*config.Config, error) {
	path, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	return config.EnsureContext(path, serverOrDefault())
}

// --- get-contexts ---

var configGetContextsJSON bool

var configGetContextsCmd = &cobra.Command{
	Use:   "get-contexts",
	Short: "List configured contexts",
	Long: `List the contexts persisted in ~/.rossoctl/config.yaml.

If the config file does not exist yet it is created, seeded with a context for
the default server. With --json the raw config is printed unchanged.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		if configGetContextsJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(cfg)
		}

		out := cmd.OutOrStdout()
		w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "CURRENT\tNAME\tSERVER\tTOKEN")
		for _, c := range cfg.Contexts {
			marker := ""
			if c.Name == cfg.CurrentContext {
				marker = "*"
			}
			token := "<none>"
			if c.BearerToken != "" {
				token = "<set>"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", marker, c.Name, c.Server, token)
		}
		return w.Flush()
	},
}

// --- use-context ---

var configUseContextCmd = &cobra.Command{
	Use:   "use-context <name>",
	Short: "Set the current context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		if err := cfg.SetCurrent(name); err != nil {
			return err
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		cmd.Printf("Switched to context %q.\n", name)
		return nil
	},
}

// --- create-context ---

var (
	createContextName   string
	createContextServer string
	createContextToken  string
)

var configCreateContextCmd = &cobra.Command{
	Use:   "create-context",
	Short: "Create a context and make it current",
	Long: `Create (or replace) a named context and make it the current context.

--server sets the context's server URI; if omitted, the global --server value
is used, falling back to the built-in default. --bearer-token is optional.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if createContextName == "" {
			return fmt.Errorf("--name is required")
		}

		serverURI := createContextServer
		if serverURI == "" {
			serverURI = serverOrDefault()
		}

		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		cfg.Upsert(config.Context{
			Name:        createContextName,
			Server:      serverURI,
			BearerToken: createContextToken,
		})
		// Creating a context makes it the current one.
		if err := cfg.SetCurrent(createContextName); err != nil {
			return err
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		cmd.Printf("Created context %q and set it as current.\n", createContextName)
		return nil
	},
}

func init() {
	configCmd := newGroup("config", "Manage rossoctl contexts")

	configGetContextsCmd.Flags().BoolVar(&configGetContextsJSON, "json", false, "print the raw config as JSON")

	configCreateContextCmd.Flags().StringVar(&createContextName, "name", "", "name of the context (required)")
	configCreateContextCmd.Flags().StringVar(&createContextServer, "server", "", "server URI for the context (default: global --server or built-in default)")
	configCreateContextCmd.Flags().StringVar(&createContextToken, "bearer-token", "", "optional bearer token for the context")

	configCmd.AddCommand(
		configGetContextsCmd,
		configUseContextCmd,
		configCreateContextCmd,
	)
	rootCmd.AddCommand(configCmd)
}
