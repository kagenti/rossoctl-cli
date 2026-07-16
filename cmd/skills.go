package cmd

func init() {
	skillsCmd := newGroup("skills", "Manage skills")
	skillsCmd.AddCommand(
		newLeaf("list", "List skills"),
		newLeaf("publish [path]", "Publish a skill"),
	)
	rootCmd.AddCommand(skillsCmd)
}
