package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestMain isolates HOME to a throwaway directory for the whole cmd test
// binary, so no test can create or mutate the real ~/.rossoctl/config.yaml
// when a command resolves its server via the context config.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rossoctl-cmd-test-home")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", dir)
	// Never spawn a real browser during the device-login tests, and don't
	// actually sleep between token polls.
	browserOpener = func(string) error { return nil }
	deviceflowSleep = func(time.Duration) {}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// execute runs the given args against the root command tree and returns
// whatever was written to stdout/stderr plus any error. Cobra shares global
// command state, so tests capture output via SetOut rather than relying on
// os.Stdout.
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()

	// Cobra flag values are stored in package-level globals bound once in
	// init(); they persist across Execute calls in the same test binary.
	// Reset every flag to its default so tests don't leak state into each
	// other (e.g. --json set by one test affecting the next).
	resetFlags(rootCmd)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	err := rootCmd.Execute()
	return buf.String(), err
}

// executeSplit is like execute but captures stdout and stderr separately, so
// tests can assert that verbose logging lands on stderr without polluting the
// stdout results (e.g. --json).
func executeSplit(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	resetFlags(rootCmd)

	var outBuf, errBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	err = rootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// resetFlags restores flag state between Execute calls in the same test
// binary. Flag values are stored in package-level globals bound once in
// init(); Cobra never resets them, so a flag set by one test would otherwise
// leak into the next.
//
// Scalar flags are restored via Set(DefValue), but slice flags (pflag
// StringSlice) need SliceValue.Replace: their Set appends after the first call
// and Set("[]") inserts a literal element rather than clearing. Every flag's
// Changed bit is then cleared.
func resetFlags(cmd *cobra.Command) {
	clear := func(f *pflag.Flag) {
		if sv, isSlice := f.Value.(pflag.SliceValue); isSlice {
			// Replace resets the value type's internal state (unlike
			// Set("[]"), which appends a literal element).
			_ = sv.Replace([]string{})
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	cmd.Flags().VisitAll(clear)
	cmd.PersistentFlags().VisitAll(clear)
	for _, sub := range cmd.Commands() {
		resetFlags(sub)
	}
}

func TestVersionCommand(t *testing.T) {
	out, err := execute(t, "version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "rossoctl") {
		t.Errorf("version output = %q, want it to contain %q", out, "rossoctl")
	}
}

// unimplementedCommands lists every documented leaf command, addressed by its
// full path from the root. Each must print UNIMPLEMENTED and exit without
// error.
var unimplementedCommands = [][]string{
	{"apply"},
	{"install"},
	// "login" is implemented (sets the current context's token); tested in login_test.go.
	{"status"},
	{"uninstall"},
	{"agents", "add-skill"},
	{"agents", "chat"},
	{"agents", "connect"},
	// "agents delete" is implemented (DELETE /agents/<ns>/<name>); tested in
	// agents_delete_test.go.
	// "agents import" has its own from-image/from-source subcommands (tested
	// in agents_import_test.go).
	{"agents", "describe"},
	{"agents", "hibernate"},
	// "agents list" is implemented (fetches GET /agents) and tested separately.
	{"agents", "promote"},
	{"agents", "scale"},
	{"agents", "wake"},
	{"gateway", "routes"},
	{"images", "preload"},
	{"skills", "list"},
	{"skills", "publish"},
	// "tools list" is implemented (fetches GET /tools) and tested separately;
	// "tools import" has from-image/from-source subcommands (tested in
	// tools_import_test.go).
	{"tools", "delete"},
	{"ui", "open"},
}

// TestUnimplementedDescriptionsPrefixed verifies that stub commands advertise
// their status in subcommand listings: their Short description begins with
// "UNIMPLEMENTED", while implemented commands do not.
func TestUnimplementedDescriptionsPrefixed(t *testing.T) {
	// A stub leaf: `agents describe`.
	if c, _, _ := rootCmd.Find([]string{"agents", "describe"}); c == nil {
		t.Fatal("agents describe not found")
	} else if !strings.HasPrefix(c.Short, "UNIMPLEMENTED") {
		t.Errorf("stub `agents describe` Short = %q, want UNIMPLEMENTED prefix", c.Short)
	}

	// A stub import subcommand: `agents import from-source`.
	if c, _, _ := rootCmd.Find([]string{"agents", "import", "from-source"}); c == nil {
		t.Fatal("agents import from-source not found")
	} else if !strings.HasPrefix(c.Short, "UNIMPLEMENTED") {
		t.Errorf("stub `agents import from-source` Short = %q, want UNIMPLEMENTED prefix", c.Short)
	}

	// An implemented command must NOT be prefixed.
	if c, _, _ := rootCmd.Find([]string{"agents", "list"}); c == nil {
		t.Fatal("agents list not found")
	} else if strings.HasPrefix(c.Short, "UNIMPLEMENTED") {
		t.Errorf("implemented `agents list` Short = %q, should not be prefixed", c.Short)
	}

	// The listing shown by `rossoctl agents` must surface the prefix.
	out, err := execute(t, "agents")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "UNIMPLEMENTED:") {
		t.Errorf("`agents` help does not surface UNIMPLEMENTED status:\n%s", out)
	}
}

func TestUnimplementedCommandsPrintPlaceholder(t *testing.T) {
	for _, path := range unimplementedCommands {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			out, err := execute(t, path...)
			if err != nil {
				t.Fatalf("%v: unexpected error: %v", path, err)
			}
			if !strings.Contains(out, "UNIMPLEMENTED") {
				t.Errorf("%v output = %q, want it to contain %q", path, out, "UNIMPLEMENTED")
			}
		})
	}
}

// TestGroupsAreNotRunnable verifies that group commands (agents, config, ...)
// show help instead of executing an UNIMPLEMENTED stub when invoked with no
// subcommand. The help listing may mention "UNIMPLEMENTED:" in a subcommand's
// description, so we check that the standalone placeholder line was not
// printed rather than that the substring is absent.
func TestGroupsAreNotRunnable(t *testing.T) {
	groups := []string{"agents", "config", "gateway", "images", "namespaces", "skills", "tools", "ui"}
	for _, g := range groups {
		t.Run(g, func(t *testing.T) {
			out, err := execute(t, g)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", g, err)
			}
			for line := range strings.SplitSeq(out, "\n") {
				if strings.TrimSpace(line) == "UNIMPLEMENTED" {
					t.Errorf("%s executed a stub (printed UNIMPLEMENTED); expected help output", g)
				}
			}
			if !strings.Contains(out, "Available Commands") {
				t.Errorf("%s output = %q, want help with %q", g, out, "Available Commands")
			}
		})
	}
}
