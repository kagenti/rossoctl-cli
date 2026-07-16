package cmd

import (
	"strings"
	"testing"
)

func TestAgentsImportIsGroup(t *testing.T) {
	// Running the group with no subcommand shows help, not the standalone
	// UNIMPLEMENTED placeholder line (the subcommand descriptions do carry an
	// "UNIMPLEMENTED:" prefix, which is expected).
	out, err := execute(t, "agents", "import")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == "UNIMPLEMENTED" {
			t.Errorf("`agents import` executed a stub; expected help:\n%s", out)
		}
	}
	for _, sub := range []string{"from-image", "from-source"} {
		if !strings.Contains(out, sub) {
			t.Errorf("`agents import` help missing subcommand %q:\n%s", sub, out)
		}
	}
	// The old `deploy` name must be gone from the command tree.
	if c, _, _ := rootCmd.Find([]string{"agents", "deploy"}); c != nil && c.Name() == "deploy" {
		t.Error("`agents deploy` should no longer exist")
	}
}

func TestAgentsImportSubcommandsUnimplemented(t *testing.T) {
	cases := [][]string{
		{"agents", "import", "from-image"},
		{"agents", "import", "from-source"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := execute(t, args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, "UNIMPLEMENTED") {
				t.Errorf("%v output = %q, want UNIMPLEMENTED", args, out)
			}
		})
	}
}

func TestAgentsImportFromImageFlags(t *testing.T) {
	// All documented flags must parse without error.
	out, err := execute(t, "agents", "import", "from-image",
		"--namespace", "team2",
		"--name", "orders",
		"--envVarsURL", "http://example.com/env",
		"--containerImage", "ghcr.io/x/y:latest",
		"--imagePullSecret", "regcred",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "UNIMPLEMENTED") {
		t.Errorf("output = %q, want UNIMPLEMENTED", out)
	}
}

func TestAgentsImportFromSourceFlags(t *testing.T) {
	out, err := execute(t, "agents", "import", "from-source",
		"--name", "orders",
		"--gitUrl", "https://github.com/x/y",
		"--gitPath", "agents/orders",
		"--gitBranch", "dev",
		"--envVarsURL", "http://example.com/env",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "UNIMPLEMENTED") {
		t.Errorf("output = %q, want UNIMPLEMENTED", out)
	}
}

func TestAgentsImportNamespaceDefault(t *testing.T) {
	for _, sub := range []string{"from-image", "from-source"} {
		t.Run(sub, func(t *testing.T) {
			// Look up the resolved --namespace default from the command tree.
			cmd, _, err := rootCmd.Find([]string{"agents", "import", sub})
			if err != nil {
				t.Fatalf("could not find command: %v", err)
			}
			f := cmd.Flags().Lookup("namespace")
			if f == nil {
				t.Fatalf("%s has no --namespace flag", sub)
			}
			if f.DefValue != "team1" {
				t.Errorf("%s --namespace default = %q, want %q", sub, f.DefValue, "team1")
			}
		})
	}
}

func TestAgentsImportFromSourceGitBranchDefault(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"agents", "import", "from-source"})
	if err != nil {
		t.Fatalf("could not find command: %v", err)
	}
	f := cmd.Flags().Lookup("gitBranch")
	if f == nil {
		t.Fatal("from-source has no --gitBranch flag")
	}
	if f.DefValue != "main" {
		t.Errorf("--gitBranch default = %q, want %q", f.DefValue, "main")
	}
}
