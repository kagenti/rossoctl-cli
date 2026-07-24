package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rossoctl/rossoctl-cli/internal/apiclient"
	"github.com/rossoctl/rossoctl-cli/internal/rossoctlclient"
)

var statusJSON bool

// statusData aggregates the three API responses that back the web UI's
// Current Session and Platform Status panels. It is what --json prints.
type statusData struct {
	Session  *apiclient.AuthStatus     `json:"session"`
	User     *apiclient.UserInfo       `json:"user"`
	Platform *apiclient.PlatformStatus `json:"platform"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show platform status",
	Long: `Show the current session and platform status, mirroring the web UI's
admin page.

The current session is read from GET <server>/auth/status and GET <server>/auth/me;
the platform status is read from GET <server>/config/platform-status.

By default the information is printed as single-column text, laid out in the
same sections as the web UI (Current Session, Platform Status). With --json the
raw data from the API is printed as JSON instead of human-formatted text.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newClient(cmd)
		if err != nil {
			return err
		}

		data, err := gatherStatus(cmd.Context(), client)
		if err != nil {
			return err
		}

		if statusJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(data)
		}

		printStatus(cmd.OutOrStdout(), data)
		return nil
	},
}

// gatherStatus fetches the three status endpoints the admin page reads.
func gatherStatus(ctx context.Context, client rossoctlclient.Rossoctl) (*statusData, error) {
	session, err := client.GetAuthStatus(ctx)
	if err != nil {
		return nil, err
	}
	user, err := client.GetUserInfo(ctx)
	if err != nil {
		return nil, err
	}
	platform, err := client.GetPlatformStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &statusData{Session: session, User: user, Platform: platform}, nil
}

// printStatus renders the status as single-column text, mirroring the web UI's
// Current Session and Platform Status panels.
func printStatus(out io.Writer, d *statusData) {
	// Current Session.
	section(out, "Current Session")
	rows := newRows()
	rows.add("Authentication", enabledLabel(d.Session.Enabled))
	rows.add("Status", authStatusLabel(d.Session.Authenticated))
	// The UI shows user details only when a user is present. GET /auth/me
	// returns a guest user when unauthenticated, so gate on Authenticated.
	if d.User != nil && d.User.Authenticated {
		rows.add("Username", d.User.Username)
		if d.User.Email != "" {
			rows.add("Email", d.User.Email)
		}
		if len(d.User.Roles) > 0 {
			rows.add("Roles", strings.Join(d.User.Roles, ", "))
		}
	}
	rows.flush(out)

	// Platform Status: components, then Registry & Build.
	section(out, "Platform Status")

	if len(d.Platform.Components) == 0 {
		fmt.Fprintln(out, "  No components reported.")
	} else {
		comps := newRows()
		for _, c := range d.Platform.Components {
			comps.add(c.Name, orDefault(c.Status, "Unknown"))
		}
		comps.flush(out)
	}

	section(out, "Registry & Build")
	reg := newRows()
	reg.add("ClusterBuildStrategy", presentLabel(d.Platform.Registry.ClusterBuildStrategyPresent))
	if len(d.Platform.Registry.ClusterBuildStrategies) > 0 {
		reg.add("Strategies", strings.Join(d.Platform.Registry.ClusterBuildStrategies, ", "))
	}
	reg.add("Registry endpoint", orDefault(d.Platform.Registry.RegistryEndpoint, "—"))
	reg.flush(out)
}

// enabledLabel renders auth enablement as the UI's Enabled/Disabled label.
func enabledLabel(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Disabled"
}

// authStatusLabel renders the session state as the UI's Authenticated/Guest label.
func authStatusLabel(authenticated bool) string {
	if authenticated {
		return "Authenticated"
	}
	return "Guest"
}

// presentLabel renders a boolean presence as the UI's Present/Missing label.
func presentLabel(present bool) string {
	if present {
		return "Present"
	}
	return "Missing"
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "print the raw data from the API as JSON instead of human-formatted text")
	rootCmd.AddCommand(statusCmd)
}
