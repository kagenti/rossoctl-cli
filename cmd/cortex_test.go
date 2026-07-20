package cmd

import (
	"strings"
	"testing"

	"github.com/kagenti/rossoctl-cli/internal/config"
)

// loadTestConfig reads the config written under the isolated HOME so tests can
// assert on the persisted contexts.
func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()
	path, err := config.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

// TestCortexStartCreatesDefaultContext verifies that `cortex start` with no
// flags creates a cortex-typed context named "default" and makes it current.
func TestCortexStartCreatesDefaultContext(t *testing.T) {
	isolateHome(t)

	out, err := execute(t, "cortex", "start")
	if err != nil {
		t.Fatalf("cortex start: %v", err)
	}
	if !strings.Contains(out, `cortex "default"`) {
		t.Errorf("expected start message to name the default cortex:\n%s", out)
	}

	cfg := loadTestConfig(t)
	ctx, ok := cfg.Get("default")
	if !ok {
		t.Fatalf("expected a context named %q to be created", "default")
	}
	if ctx.Type != config.TypeCortex {
		t.Errorf("context type = %q, want %q", ctx.Type, config.TypeCortex)
	}
	if cfg.CurrentContext != "default" {
		t.Errorf("current context = %q, want %q", cfg.CurrentContext, "default")
	}
	// start seeds the namespace from the cortex backend's first namespace,
	// which FileClient reports as "default".
	if ctx.Namespace != "default" {
		t.Errorf("namespace = %q, want %q (seeded from the cortex backend)", ctx.Namespace, "default")
	}
}

// TestCortexStopDoesNotSeedNamespace verifies that only `start` seeds the
// namespace on creation: `stop` creating the context leaves it blank.
func TestCortexStopDoesNotSeedNamespace(t *testing.T) {
	isolateHome(t)

	if _, err := execute(t, "cortex", "stop"); err != nil {
		t.Fatalf("cortex stop: %v", err)
	}
	cfg := loadTestConfig(t)
	ctx, ok := cfg.Get("default")
	if !ok {
		t.Fatalf("expected a context named %q to be created", "default")
	}
	if ctx.Namespace != "" {
		t.Errorf("namespace = %q, want empty (stop should not seed)", ctx.Namespace)
	}
}

// TestCortexCommandsUseCortexFlag verifies --cortex selects/creates a
// cortex-typed context of that name across start/stop/status.
func TestCortexCommandsUseCortexFlag(t *testing.T) {
	for _, sub := range []string{"start", "stop", "status"} {
		t.Run(sub, func(t *testing.T) {
			isolateHome(t)

			out, err := execute(t, "cortex", sub, "--cortex", "myctx")
			if err != nil {
				t.Fatalf("cortex %s --cortex myctx: %v", sub, err)
			}
			if !strings.Contains(out, `"myctx"`) {
				t.Errorf("expected message to name the myctx cortex:\n%s", out)
			}

			cfg := loadTestConfig(t)
			ctx, ok := cfg.Get("myctx")
			if !ok {
				t.Fatalf("expected a context named %q to be created", "myctx")
			}
			if ctx.Type != config.TypeCortex {
				t.Errorf("context type = %q, want %q", ctx.Type, config.TypeCortex)
			}
			if cfg.CurrentContext != "myctx" {
				t.Errorf("current context = %q, want %q", cfg.CurrentContext, "myctx")
			}
		})
	}
}

// TestCortexContextOverrideMustExist verifies that --context naming a
// nonexistent context errors and never creates one.
func TestCortexContextOverrideMustExist(t *testing.T) {
	isolateHome(t)

	if _, err := execute(t, "cortex", "status", "--context", "ghost"); err == nil {
		t.Error("cortex status --context ghost should error on an unknown context")
	}
	cfg := loadTestConfig(t)
	if _, ok := cfg.Get("ghost"); ok {
		t.Error("--context should not create a context")
	}
}

// TestCortexContextOverrideWrongType verifies that pointing --context at a
// non-cortex context is rejected.
func TestCortexContextOverrideWrongType(t *testing.T) {
	isolateHome(t)

	// Create a k8s context, then try to drive cortex against it.
	if _, err := execute(t, "config", "create-context",
		"--name", "k8sctx", "--server", "http://x/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	_, err := execute(t, "cortex", "status", "--context", "k8sctx")
	if err == nil {
		t.Fatal("cortex status against a k8s context should error")
	}
	if !strings.Contains(err.Error(), string(config.TypeCortex)) {
		t.Errorf("error should mention the cortex type: %v", err)
	}
}
