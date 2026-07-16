package cmd

func init() {
	gatewayCmd := newGroup("gateway", "Manage the MCP gateway")
	gatewayCmd.AddCommand(
		newLeaf("routes", "List gateway routes"),
	)
	rootCmd.AddCommand(gatewayCmd)
}
