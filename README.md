# rossoctl-cli

A command-line interface for Rossoctl, built with [Cobra](https://github.com/spf13/cobra).

## Install

The `downloadRossoctl` script downloads the release archive for your platform
extracts it, and installs the binary at
`$HOME/.config/rossoctl/rossoctl`:

```sh
curl -fsSL https://raw.githubusercontent.com/rossoctl/rossoctl-cli/main/downloadRossoctl | sh
PATH=$PATH:$HOME/.config/rossoctl
# alternately, sudo mv $HOME/.config/rossoctl /usr/local/bin
```

## Quick usage, for shared OpenShift Rossoctl API servers

```sh
# (Choose w3id for shared cluster login)
rossoctl --server https://rossoctl-ui-rossoctl-system.apps.ykt3.hcp.res.ibm.com/api/v1 login
rossoctl agents list
```

## Quick usage, for existing Kind cluster Rossoctl API server

```sh
rossoctl login
rossoctl agents list
```

## Quick usage, for Alek-style Docker Rossoctl Cortext

```sh
rossoctl doctor
rossoctl cortex start
# (under construction)
```

## Usage

```sh
rossoctl --help
rossoctl version
rossoctl agents --help

# Manage contexts (persisted in ~/.config/rossoctl/config.yaml, kubectl-style)
rossoctl config get-contexts                    # created + seeded on first use
rossoctl config create-context --name dev \
    --server http://my-host:8080/api/v1/ --namespace team1 --bearer-token <token>   # becomes current
rossoctl config use-context dev
rossoctl config set-context --namespace team1   # set namespace on current context (warns if unknown to server)
rossoctl config set-context --namespace team1 --server http://other:8080/api/v1/   # also replace the server
rossoctl config set-context --name prod          # rename the current context (updates the current reference)
rossoctl login --token <token>                  # set the token on the current context directly
rossoctl login                                  # or: OAuth device flow against the server's Keycloak
rossoctl login --server http://host:8080/api/v1/ --token <token>   # target the context for that host (create if absent), make it current

# Show the server's auth configuration (GET <server>/auth/config)
rossoctl auth-config
rossoctl auth-config --json
rossoctl --server http://my-host:8080/api/v1/ auth-config

# List agents (GET <server>/agents)
rossoctl agents list                            # single namespace: agents --namespace, else current context
rossoctl agents --namespace team2 list          # list one specific namespace
rossoctl agents list --all-namespaces           # -A: discover via GET /namespaces, list across all
rossoctl agents list --all-namespaces --json    # each namespace's response, separated by ---

# Show one agent (GET <server>/agents/<namespace>/<name>)
rossoctl agents get orders                      # single-column text, laid out like the web detail page
rossoctl agents get orders --json               # raw JSON

# Delete an agent (DELETE <server>/agents/<namespace>/<name>)
rossoctl agents delete orders

# Import an agent from a container image (POST <server>/agents)
rossoctl agents import from-image --name orders --containerImage ghcr.io/x/y:latest
rossoctl agents import --deployment-type sandbox from-image \
    --name orders --containerImage ghcr.io/x/y:latest --imagePullSecret regcred \
    --envVarsURL https://example.com/orders.env   # newline-separated key=value

# `agents --namespace` overrides the context's namespace for any agents subcommand
rossoctl agents --namespace team2 get orders    # -> GET /agents/team2/orders

# `agents --context` uses a named context instead of the current one (its server, token, namespace)
rossoctl agents --context prod get orders
rossoctl agents --context prod --namespace teamX list   # --namespace still overrides the context's namespace

# Tools mirror the agents commands, against the /tools endpoint.
# --namespace, --context, --all-namespaces (-A), and --json behave as for agents.
rossoctl tools list                              # single namespace (context, or --namespace)
rossoctl tools list --all-namespaces             # discover and list across all
rossoctl tools --namespace team2 list --json
rossoctl tools delete weather-mcp                # DELETE /tools/<namespace>/weather-mcp
rossoctl tools import from-image --name weather-mcp --containerImage ghcr.io/x/y:latest  # POST /tools
rossoctl tools import --deployment-type statefulset from-image \
    --name weather-mcp --containerImage ghcr.io/x/y:latest --envVarsURL https://example.com/tool.env
# --ports sets service ports as name:port:targetPort[:protocol] (default http:9090:9090:TCP); a bare "port" = http:port:port:TCP
rossoctl tools import from-image --name weather-mcp --containerImage ghcr.io/x/y:latest --ports grpc:9000:9001:TCP,8080

# List namespaces (GET <server>/namespaces)
rossoctl namespaces list
rossoctl namespaces list --all      # include non-rossoctl-enabled namespaces
rossoctl namespaces list --json

# Log the underlying REST requests to stderr
rossoctl -v agents list
```

## Full docs

See [the documentation](./docs)
