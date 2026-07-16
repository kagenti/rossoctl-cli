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
│   ├── toplevel.go             # apply, install, login, status, uninstall
│   ├── agents.go               # `rossoctl agents ...` (`list` fetches GET /agents)
│   ├── config.go               # `rossoctl config` (shows server auth config)
│   ├── gateway.go              # `rossoctl gateway ...`
│   ├── images.go               # `rossoctl images ...`
│   ├── namespaces.go           # `rossoctl namespaces ...` (`list` fetches GET /namespaces)
│   ├── skills.go               # `rossoctl skills ...`
│   ├── tools.go                # `rossoctl tools ...` (`list` fetches GET /tools)
│   └── ui.go                   # `rossoctl ui ...`
├── internal/                   # Private application logic (not importable externally)
│   ├── apiclient/              # HTTP client for the Rossoctl backend API
│   └── buildinfo/              # Version metadata formatting
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

# Show the server's auth configuration (GET <server>/auth/config)
rossoctl config
rossoctl config --json
rossoctl --server http://my-host:8080/api/v1/ config

# List agents (GET <server>/agents, one request per namespace)
rossoctl agents list                            # discovers namespaces via GET /namespaces, lists agents in each
rossoctl agents list --namespaces team1,team2   # comma-separated
rossoctl agents list -n team1 -n team2          # or repeated
rossoctl agents list --namespaces team1,team2 --json   # each response, separated by ---

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

The global `--server` flag sets the API root (default
`http://kagenti-ui.localtest.me:8080/api/v1/`) and must appear before the
subcommand. The global `--verbose`/`-v` flag logs each REST request (method,
URL, status, timing) to stderr.

The command tree mirrors the subcommands referenced in the Rossoctl docs
(`agents`, `config`, `gateway`, `images`, `namespaces`, `skills`, `tools`, `ui`,
plus the top-level `apply`, `install`, `login`, `status`, `uninstall`). The
`config`, `agents list`, `tools list`, and `namespaces list` commands are
implemented; other leaf commands currently print `UNIMPLEMENTED` as a
placeholder.
