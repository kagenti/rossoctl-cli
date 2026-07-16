package cmd

import "github.com/spf13/cobra"

// unimplementedPrefix is prepended to the Short description of every stub
// command so its status is visible in help/subcommand listings (e.g. the
// output of `rossoctl` or `rossoctl agents`).
const unimplementedPrefix = "UNIMPLEMENTED: "

// unimplementedRunE is the RunE used by every leaf command that is documented
// but not yet built. It simply prints a placeholder so the command tree is
// navigable while the real behavior is filled in later.
func unimplementedRunE(cmd *cobra.Command, _ []string) error {
	cmd.Println("UNIMPLEMENTED")
	return nil
}

// newGroup returns a parent command that only groups subcommands. Running it
// with no subcommand prints its help rather than "UNIMPLEMENTED".
func newGroup(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
	}
}

// newLeaf returns a runnable command whose implementation is a placeholder.
//
// Because these are stubs, they accept any flags and positional arguments so
// that the invocations shown in the documentation (e.g. `install --local`,
// `agent deploy foo --namespace team1`) run and print the placeholder rather
// than failing on an unknown flag. Real flag definitions replace this when a
// command is implemented.
func newLeaf(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Short:              unimplementedPrefix + short,
		RunE:               unimplementedRunE,
		DisableFlagParsing: true,
	}
}
