package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormArch(t *testing.T) {
	cases := map[string]string{
		"arm64":   "arm64",
		"aarch64": "arm64",
		"x86_64":  "amd64",
		"amd64":   "amd64",
		"AMD64":   "amd64",
		"riscv64": "riscv64",
	}
	for in, want := range cases {
		if got := normArch(in); got != want {
			t.Errorf("normArch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDirWritable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")
	if !dirWritable(dir) {
		t.Errorf("dirWritable(%q) = false, want true", dir)
	}
	// The probe file must be cleaned up.
	if _, err := os.Stat(filepath.Join(dir, ".doctor-write-test")); !os.IsNotExist(err) {
		t.Errorf("probe file left behind, stat err = %v", err)
	}
}

func TestFindCredential(t *testing.T) {
	credsDir := t.TempDir()

	// No files, no env → none.
	for _, n := range credentialNames {
		t.Setenv(n, "")
	}
	if got := findCredential(credsDir); got != "" {
		t.Errorf("findCredential with nothing set = %q, want empty", got)
	}

	// Env var wins when no file is present.
	t.Setenv("OPENAI_API_KEY", "sk-test")
	if got := findCredential(credsDir); got != "env $OPENAI_API_KEY" {
		t.Errorf("findCredential (env) = %q", got)
	}

	// A credential file takes priority over env, and the file listed first in
	// credentialNames wins.
	if err := os.WriteFile(filepath.Join(credsDir, "LITELLM_API_KEY"), []byte("key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := findCredential(credsDir)
	if !strings.HasPrefix(got, "file ") || !strings.HasSuffix(got, "LITELLM_API_KEY") {
		t.Errorf("findCredential (file) = %q, want the LITELLM_API_KEY file", got)
	}
}

func TestStateUpstream(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "rossocortex-state.json")

	// Missing file → "".
	if got := stateUpstream(stateFile); got != "" {
		t.Errorf("stateUpstream(missing) = %q, want empty", got)
	}
	// Valid file with upstream.
	if err := os.WriteFile(stateFile, []byte(`{"upstream":"http://up/","port":8185}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := stateUpstream(stateFile); got != "http://up/" {
		t.Errorf("stateUpstream = %q, want %q", got, "http://up/")
	}
	// Malformed JSON → "".
	if err := os.WriteFile(stateFile, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := stateUpstream(stateFile); got != "" {
		t.Errorf("stateUpstream(bad json) = %q, want empty", got)
	}
}

func TestConfigDirNote(t *testing.T) {
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if note := configDirNote("/d"); !strings.Contains(note, "default (~/.config)") {
		t.Errorf("configDirNote default = %q", note)
	}
	t.Setenv("XDG_CONFIG_HOME", "/x")
	if note := configDirNote("/d"); !strings.Contains(note, "XDG_CONFIG_HOME override") {
		t.Errorf("configDirNote XDG = %q", note)
	}
	t.Setenv("ROSSOCORTEX_CONFIG_DIR", "/r")
	if note := configDirNote("/d"); !strings.Contains(note, "ROSSOCORTEX_CONFIG_DIR override") {
		t.Errorf("configDirNote ROSSOCORTEX = %q", note)
	}
}

func TestReportDoctorTallyAndExit(t *testing.T) {
	// A failing check yields a non-nil error and is tallied under "failed".
	var buf bytes.Buffer
	err := reportDoctor(&buf, []doctorCheck{
		{boolPtr(true), "pass", "", ""},
		{nil, "warn", "", "note text"},
		{boolPtr(false), "fail", "", "fix text"},
	})
	if err == nil {
		t.Error("reportDoctor with a failing check should return an error")
	}
	out := buf.String()
	if !strings.Contains(out, "1 passed, 1 warnings, 1 failed") {
		t.Errorf("tally line missing:\n%s", out)
	}
	if !strings.Contains(out, "fix: fix text") {
		t.Errorf("fix hint missing:\n%s", out)
	}
	if !strings.Contains(out, "note: note text") {
		t.Errorf("note hint missing:\n%s", out)
	}

	// All-pass yields no error and the "Ready" line.
	buf.Reset()
	if err := reportDoctor(&buf, []doctorCheck{{boolPtr(true), "ok", "", ""}}); err != nil {
		t.Errorf("reportDoctor all-pass returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "Ready.") {
		t.Errorf("expected Ready line:\n%s", buf.String())
	}
}

func TestCortexDoctorRuns(t *testing.T) {
	isolateHome(t)
	// The doctor command prints its banner and a tally regardless of pass/fail;
	// it may return an error (e.g. no container runtime), which is fine here.
	out, _ := execute(t, "cortex", "doctor")
	if !strings.Contains(out, "environment preflight") {
		t.Errorf("doctor output missing banner:\n%s", out)
	}
	if !strings.Contains(out, "passed,") {
		t.Errorf("doctor output missing tally:\n%s", out)
	}
}
