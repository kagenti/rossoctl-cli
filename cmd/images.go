package cmd

func init() {
	imagesCmd := newGroup("images", "Manage platform images")
	imagesCmd.AddCommand(
		newLeaf("preload", "Preload images into the cluster"),
	)
	rootCmd.AddCommand(imagesCmd)
}
