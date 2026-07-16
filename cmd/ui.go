package cmd

func init() {
	uiCmd := newGroup("ui", "Interact with the rossoctl UI")
	uiCmd.AddCommand(
		newLeaf("open", "Open the rossoctl UI"),
	)
	rootCmd.AddCommand(uiCmd)
}
