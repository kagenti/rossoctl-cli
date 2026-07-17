# rossoctl-cli

A command-line interface for Rossoctl, built with [Cobra](https://github.com/spf13/cobra).

## Layout

This project follows the standard Go CLI layout:

```
.
├── main.go                     # Thin entry point; calls cmd.Execute()
├── cmd/                        # Cobra command tree (grouped by command)
│   ├── root.go                 # Root command + Execute() + persistent flags
│   ├── version.go              # `rossoctl version`
│   ├── unimplemented.go        # newGroup/newLeaf helpers + UNIMPLEMENTED stub
│   ├── toplevel.go             # apply, install, status, uninstall
│   ├── login.go                # `rossoctl login` (--token or OAuth device flow)
│   ├── agents.go               # `rossoctl agents ...` (`list` fetches GET /agents)
│   ├── authconfig.go           # `rossoctl auth-config` (shows server auth config)
│   ├── config.go               # `rossoctl config ...` (context management)
│   ├── gateway.go              # `rossoctl gateway ...`
│   ├── images.go               # `rossoctl images ...`
│   ├── namespaces.go           # `rossoctl namespaces ...` (`list` fetches GET /namespaces)
│   ├── skills.go               # `rossoctl skills ...`
│   ├── tools.go                # `rossoctl tools ...` (`list` fetches GET /tools)
│   └── ui.go                   # `rossoctl ui ...`
├── internal/                   # Private application logic (not importable externally)
│   ├── apiclient/              # HTTP client for the Rossoctl backend API
│   ├── buildinfo/              # Version metadata formatting
│   ├── config/                 # ~/.rossoctl/config.yaml context persistence
│   └── deviceflow/             # OAuth 2.0 device authorization grant (Keycloak)
├── Makefile
└── go.mod
```

Design principles:

- **`main.go` stays trivial** — it only calls `cmd.Execute()`.
- **`cmd/` handles the CLI surface** — flag parsing, help text, and wiring.
  Each command lives in its own file and registers itself with `rootCmd` in
  `init()`.
- **`internal/` holds the real logic** — packages there are free of Cobra and
  of I/O, so they can be unit-tested directly. `internal/` also prevents other
  modules from importing this code.

## Build

```sh
make build      # -> ./bin/rossoctl (version info injected via -ldflags)
make install    # install into $GOBIN
make test       # go test ./...
```

## Usage

```sh
rossoctl --help
rossoctl version
rossoctl agents --help

# Manage contexts (persisted in ~/.rossoctl/config.yaml, kubectl-style)
rossoctl config get-contexts                    # created + seeded on first use
rossoctl config create-context --name dev \
    --server http://my-host:8080/api/v1/ --namespace team1 --bearer-token <token>   # becomes current
rossoctl config use-context dev
rossoctl config set-context --namespace team1   # set namespace on current context (warns if unknown to server)
rossoctl login --token <token>                  # set the token on the current context directly
rossoctl login                                  # or: OAuth device flow against the server's Keycloak

# Show the server's auth configuration (GET <server>/auth/config)
rossoctl auth-config
rossoctl auth-config --json
rossoctl --server http://my-host:8080/api/v1/ auth-config

# List agents (GET <server>/agents, one request per namespace)
rossoctl agents list                            # discovers namespaces via GET /namespaces, lists agents in each
rossoctl agents list --namespaces team1,team2   # comma-separated
rossoctl agents list -n team1 -n team2          # or repeated
rossoctl agents list --namespaces team1,team2 --json   # each response, separated by ---

# Show one agent (GET <server>/agents/<namespace>/<name>)
rossoctl agents get orders                      # single-column text, laid out like the web detail page
rossoctl agents get orders --json               # raw JSON

# Delete an agent (DELETE <server>/agents/<namespace>/<name>)
rossoctl agents delete orders

# `agents --namespace` overrides the current context's namespace for any agents subcommand
rossoctl agents --namespace team2 get orders    # -> GET /agents/team2/orders
rossoctl agents --namespace team2 list          # list just team2 (no discovery)

# List tools (GET <server>/tools) — same options as `agents list`
rossoctl tools list
rossoctl tools list --namespaces team1,team2 --json

# List namespaces (GET <server>/namespaces)
rossoctl namespaces list
rossoctl namespaces list --all      # include non-kagenti-enabled namespaces
rossoctl namespaces list --json

# Log the underlying REST requests to stderr
rossoctl -v agents list
```

### Contexts and server resolution

Contexts are persisted in `~/.rossoctl/config.yaml` (directory `0700`, file
`0600`). Each context has a name, a server URI, an optional namespace, and an
optional bearer token.
The file is created lazily — the first command that needs it seeds a context
from the default server (`http://kagenti-ui.localtest.me:8080/api/v1/`) and
makes it current. Creating a context makes it current.

The server a command talks to is resolved as: an explicit `--server` flag wins
(and no bearer token is sent); otherwise the current context supplies both the
server URI and its bearer token. The global `--server` and `--verbose`/`-v`
flags must appear before the subcommand; `-v` logs each REST request (method,
URL, status, timing) to stderr.

### Logging in

`rossoctl login --token <token>` stores a token on the current context
directly. `rossoctl login` (no `--token`) runs the OAuth 2.0 device
authorization grant (RFC 8628): it reads `keycloak_url`, `realm`, and
`client_id` from `GET <server>/auth/config`, requests a device code from
Keycloak, prints a verification URL and one-time code (and best-effort opens a
browser), polls until you authorize, and saves the resulting bearer token on
the current context.

The command tree mirrors the subcommands referenced in the Rossoctl docs
(`agents`, `config`, `gateway`, `images`, `namespaces`, `skills`, `tools`, `ui`,
plus `auth-config` and the top-level `apply`, `install`, `login`, `status`,
`uninstall`). The
`config` context commands, `login`, `auth-config`, `agents list`,
`agents get`, `agents delete`, `tools list`, and `namespaces list` are
implemented; other leaf commands currently print `UNIMPLEMENTED` as a
placeholder.
