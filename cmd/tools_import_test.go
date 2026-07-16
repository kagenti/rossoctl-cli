package cmd

import (
	"strings"
	"testing"
)

func TestToolsImportIsGroup(t *testing.T) {
	out, err := execute(t, "tools", "import")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == "UNIMPLEMENTED" {
			t.Errorf("`tools import` executed a stub; expected help:\n%s", out)
		}
	}
	for _, sub := range []string{"from-image", "from-source"} {
		if !strings.Contains(out, sub) {
			t.Errorf("`tools import` help missing subcommand %q:\n%s", sub, out)
		}
	}
}

func TestToolsImportSubcommandsUnimplemented(t *testing.T) {
	cases := [][]string{
		{"tools", "import", "from-image"},
		{"tools", "import", "from-source"},
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

func TestToolsImportFromImageFlags(t *testing.T) {
	out, err := execute(t, "tools", "import", "from-image",
		"--namespace", "team2",
		"--name", "weather-mcp",
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

func TestToolsImportFromSourceFlags(t *testing.T) {
	out, err := execute(t, "tools", "import", "from-source",
		"--name", "weather-mcp",
		"--gitUrl", "https://github.com/x/y",
		"--gitPath", "tools/weather",
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

func TestToolsImportDefaults(t *testing.T) {
	for _, sub := range []string{"from-image", "from-source"} {
		t.Run(sub, func(t *testing.T) {
			cmd, _, err := rootCmd.Find([]string{"tools", "import", sub})
			if err != nil {
				t.Fatalf("could not find command: %v", err)
			}
			if f := cmd.Flags().Lookup("namespace"); f == nil || f.DefValue != "team1" {
				t.Errorf("%s --namespace default = %v, want team1", sub, f)
			}
		})
	}

	cmd, _, err := rootCmd.Find([]string{"tools", "import", "from-source"})
	if err != nil {
		t.Fatalf("could not find command: %v", err)
	}
	if f := cmd.Flags().Lookup("gitBranch"); f == nil || f.DefValue != "main" {
		t.Errorf("--gitBranch default = %v, want main", f)
	}
}
