package cmd

import (
	"encoding/json"
	"fmt"
	"slices"
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
		fmt.Fprintln(w, "CURRENT\tNAME\tSERVER\tNAMESPACE\tTOKEN")
		for _, c := range cfg.Contexts {
			marker := ""
			if c.Name == cfg.CurrentContext {
				marker = "*"
			}
			namespace := c.Namespace
			if namespace == "" {
				namespace = "-"
			}
			token := "<none>"
			if c.BearerToken != "" {
				token = "<set>"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", marker, c.Name, c.Server, namespace, token)
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
	createContextName      string
	createContextServer    string
	createContextNamespace string
	createContextToken     string
)

var configCreateContextCmd = &cobra.Command{
	Use:   "create-context",
	Short: "Create a context and make it current",
	Long: `Create (or replace) a named context and make it the current context.

--server sets the context's server URI; if omitted, the global --server value
is used, falling back to the built-in default. --namespace and --bearer-token
are optional.`,
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
			Namespace:   createContextNamespace,
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

// --- set-context ---

var setContextNamespace string

var configSetContextCmd = &cobra.Command{
	Use:   "set-context",
	Short: "Set the namespace on the current context",
	Long: `Set the namespace on the current context.

The value is checked against the namespaces reported by the server
(GET <server>/namespaces); if it is not among them a warning is printed, but
the namespace is set regardless.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed("namespace") {
			return fmt.Errorf("--namespace is required")
		}

		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		cur, ok := cfg.Current()
		if !ok {
			return fmt.Errorf("no current context")
		}

		// Warn (but don't fail) if the namespace is not one the server knows.
		// serverKnowsNamespace resolves the server the same way other commands
		// do, so an explicit --server (below) is used for this check too.
		if known, err := serverKnowsNamespace(cmd, setContextNamespace); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"Warning: could not verify namespace against the server: %v\n", err)
		} else if !known {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"Warning: namespace %q is not among the server's namespaces\n", setContextNamespace)
		}

		cur.Namespace = setContextNamespace
		// Replace the context's server only when --server was given explicitly
		// (not left at its default).
		if cmd.Flags().Changed("server") {
			cur.Server = server
		}

		if err := cfg.Save(); err != nil {
			return err
		}
		if cmd.Flags().Changed("server") {
			cmd.Printf("Set namespace %q and server %q on context %q.\n",
				setContextNamespace, cur.Server, cur.Name)
		} else {
			cmd.Printf("Set namespace %q on context %q.\n", setContextNamespace, cur.Name)
		}
		return nil
	},
}

// serverKnowsNamespace reports whether ns is among the namespaces the server
// returns from GET <server>/namespaces (enabled namespaces only).
func serverKnowsNamespace(cmd *cobra.Command, ns string) (bool, error) {
	client, err := newClient(cmd)
	if err != nil {
		return false, err
	}
	resp, err := client.ListNamespaces(cmd.Context(), true)
	if err != nil {
		return false, err
	}
	return slices.Contains(resp.Namespaces, ns), nil
}

func init() {
	configCmd := newGroup("config", "Manage rossoctl contexts")

	configGetContextsCmd.Flags().BoolVar(&configGetContextsJSON, "json", false, "print the raw config as JSON")

	configCreateContextCmd.Flags().StringVar(&createContextName, "name", "", "name of the context (required)")
	configCreateContextCmd.Flags().StringVar(&createContextServer, "server", "", "server URI for the context (default: global --server or built-in default)")
	configCreateContextCmd.Flags().StringVar(&createContextNamespace, "namespace", "", "optional default namespace for the context")
	configCreateContextCmd.Flags().StringVar(&createContextToken, "bearer-token", "", "optional bearer token for the context")

	configSetContextCmd.Flags().StringVar(&setContextNamespace, "namespace", "", "namespace to set on the current context (required)")

	configCmd.AddCommand(
		configGetContextsCmd,
		configUseContextCmd,
		configCreateContextCmd,
		configSetContextCmd,
	)
	rootCmd.AddCommand(configCmd)
}
