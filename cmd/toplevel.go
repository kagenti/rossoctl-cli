package cmd

// Top-level leaf commands that are documented but not yet implemented. Each
// prints "UNIMPLEMENTED" until real behavior is added.
func init() {
	rootCmd.AddCommand(
		newLeaf("apply", "Apply a resource from a file"),
		newLeaf("install", "Install the Rossoctl platform"),
		newLeaf("status", "Show platform status"),
		newLeaf("uninstall", "Uninstall the Rossoctl platform"),
	)
}
