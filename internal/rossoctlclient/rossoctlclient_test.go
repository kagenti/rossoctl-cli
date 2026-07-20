package rossoctlclient

import (
	"testing"

	"github.com/kagenti/rossoctl-cli/internal/apiclient"
	"github.com/kagenti/rossoctl-cli/internal/config"
	"github.com/kagenti/rossoctl-cli/internal/cortexclient"
)

func TestNewClientDispatchesOnType(t *testing.T) {
	tests := []struct {
		name    string
		ctxType config.Type
		want    string // "http" or "file"
	}{
		{"k8s", config.TypeK8s, "http"},
		{"cortex", config.TypeCortex, "file"},
		{"empty defaults to http", "", "http"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(&config.Context{Type: tt.ctxType, Server: "http://x/api/v1/"})
			switch tt.want {
			case "http":
				if _, ok := c.(*apiclient.Client); !ok {
					t.Errorf("type %q: got %T, want *apiclient.Client", tt.ctxType, c)
				}
			case "file":
				if _, ok := c.(*cortexclient.FileClient); !ok {
					t.Errorf("type %q: got %T, want *cortexclient.FileClient", tt.ctxType, c)
				}
			}
		})
	}
}

func TestNewClientCarriesContextFields(t *testing.T) {
	ctx := &config.Context{Type: config.TypeK8s, Server: "http://api/", BearerToken: "tok"}
	c, ok := NewClient(ctx).(*apiclient.Client)
	if !ok {
		t.Fatalf("expected *apiclient.Client, got %T", c)
	}
	if c.BaseURL != ctx.Server {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, ctx.Server)
	}
	if c.BearerToken != ctx.BearerToken {
		t.Errorf("BearerToken = %q, want %q", c.BearerToken, ctx.BearerToken)
	}

	fc, ok := NewClient(&config.Context{Type: config.TypeCortex, Name: "mycortex"}).(*cortexclient.FileClient)
	if !ok {
		t.Fatalf("expected *cortexclient.FileClient")
	}
	if fc.Name != "mycortex" {
		t.Errorf("Name = %q, want %q", fc.Name, "mycortex")
	}
}
