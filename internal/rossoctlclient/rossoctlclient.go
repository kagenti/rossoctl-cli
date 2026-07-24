// Package rossoctlclient defines the Rossoctl interface: the set of
// operations the command layer needs from a backend, independent of whether
// that backend is the live HTTP API (apiclient.Client) or a local file-backed
// implementation (cortexclient.FileClient).
//
// The interface mirrors the public methods of apiclient.Client and reuses that
// package's request/response types, so both apiclient.Client and
// cortexclient.FileClient satisfy Rossoctl without any adaptation. This package
// imports both concrete backends so NewClient can dispatch on a context's type;
// the backends therefore must not import this package (that would cycle).
package rossoctlclient

import (
	"context"

	"github.com/rossoctl/rossoctl-cli/internal/apiclient"
	"github.com/rossoctl/rossoctl-cli/internal/config"
	"github.com/rossoctl/rossoctl-cli/internal/cortexclient"
)

// Rossoctl is the backend contract used by the command layer. Its methods are
// exactly the public methods of apiclient.Client.
type Rossoctl interface {
	// GetAuthConfig fetches the server's auth configuration.
	GetAuthConfig(ctx context.Context) (*apiclient.AuthConfig, error)

	// GetAuthStatus fetches the server's authentication status (GET /auth/status).
	GetAuthStatus(ctx context.Context) (*apiclient.AuthStatus, error)

	// GetUserInfo fetches the current user (GET /auth/me); returns a guest user
	// when unauthenticated rather than erroring.
	GetUserInfo(ctx context.Context) (*apiclient.UserInfo, error)

	// GetPlatformStatus fetches aggregated platform status
	// (GET /config/platform-status).
	GetPlatformStatus(ctx context.Context) (*apiclient.PlatformStatus, error)

	// ListAgents lists agents in the given namespace (empty => server default).
	ListAgents(ctx context.Context, namespace string) (*apiclient.AgentListResponse, error)

	// GetAgent fetches a single agent by namespace and name.
	GetAgent(ctx context.Context, namespace, name string) (*apiclient.AgentDetail, error)

	// DeleteAgent deletes an agent by namespace and name.
	DeleteAgent(ctx context.Context, namespace, name string) (*apiclient.DeleteResponse, error)

	// CreateAgent creates an agent from the given request.
	CreateAgent(ctx context.Context, req *apiclient.CreateAgentRequest) (*apiclient.CreateAgentResponse, error)

	// ListTools lists tools in the given namespace (empty => server default).
	ListTools(ctx context.Context, namespace string) (*apiclient.ToolListResponse, error)

	// GetTool fetches a single tool by namespace and name.
	GetTool(ctx context.Context, namespace, name string) (*apiclient.ToolDetail, error)

	// DeleteTool deletes a tool by namespace and name.
	DeleteTool(ctx context.Context, namespace, name string) (*apiclient.DeleteResponse, error)

	// CreateTool creates a tool from the given request.
	CreateTool(ctx context.Context, req *apiclient.CreateToolRequest) (*apiclient.CreateToolResponse, error)

	// ListNamespaces lists namespaces; enabledOnly restricts to rossoctl-enabled
	// namespaces (the server default).
	ListNamespaces(ctx context.Context, enabledOnly bool) (*apiclient.NamespaceListResponse, error)
}

// Compile-time assertions that both backends implement Rossoctl.
var (
	_ Rossoctl = (*apiclient.Client)(nil)
	_ Rossoctl = (*cortexclient.FileClient)(nil)
)

// NewClient builds a Rossoctl backend for ctx, dispatching on its type:
//
//   - TypeCortex returns a file-backed cortexclient.FileClient rooted at the
//     context's server.
//   - Any other type (including the empty type, treated as api for backward
//     compatibility) returns an HTTP apiclient.Client for the context's server
//     and bearer token.
//
// Verbose request logging is not wired here: callers that want it can type-
// assert the result to *apiclient.Client and set its Logf field.
func NewClient(ctx *config.Context) Rossoctl {
	if ctx.Type == config.TypeCortex {
		return cortexclient.NewFileClient(ctx.Name)
	}
	return &apiclient.Client{
		BaseURL:     ctx.Server,
		BearerToken: ctx.BearerToken,
	}
}
