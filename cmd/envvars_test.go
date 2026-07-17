package cmd

import (
	"testing"
)

func TestParseEnvVars(t *testing.T) {
	body := "FOO=bar\n" +
		"# a comment\n" +
		"\n" +
		"  BAZ = qux value \n" +
		"EMPTY=\n" +
		"URL=http://a=b\n" // value may contain '='

	got, err := parseEnvVars(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []struct{ name, value string }{
		{"FOO", "bar"},
		{"BAZ", "qux value"},
		{"EMPTY", ""},
		{"URL", "http://a=b"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d env vars, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Name != w.name || got[i].Value != w.value {
			t.Errorf("env[%d] = %+v, want {%s %s}", i, got[i], w.name, w.value)
		}
	}
}

func TestParseEnvVarsInvalidLine(t *testing.T) {
	if _, err := parseEnvVars("FOO=bar\nnotakeyvalue\n"); err == nil {
		t.Error("expected error for a line without '='")
	}
	if _, err := parseEnvVars("=novalue\n"); err == nil {
		t.Error("expected error for an empty key")
	}
}

func TestParseEnvVarsEmpty(t *testing.T) {
	got, err := parseEnvVars("\n#only comments\n\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no env vars, got %+v", got)
	}
}
