package cmd

import (
	"strings"
	"testing"
)

func TestInstall(t *testing.T) {
	out, err := execute(t, "install")
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	for _, want := range []string{
		"To install the K8s API version, obtain the source code and",
		"git clone https://github.com/rossoctl/rossoctl.git",
		"cd rossoctl",
		"Then do one of the following",
		"./scripts/kind/setup-rossoctl.sh",
		"./scripts/k8s/setup-rossoctl.sh",
		"./scripts/ocp/setup-rossoctl.sh",
		"To run Cortex version, use `rossoctl cortex`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("install output missing %q:\n%s", want, out)
		}
	}

	// It is guidance, not the UNIMPLEMENTED placeholder.
	if strings.Contains(out, "UNIMPLEMENTED") {
		t.Errorf("install should not print UNIMPLEMENTED:\n%s", out)
	}
}
