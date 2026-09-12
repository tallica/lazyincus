# lazyincus

> [!WARNING]
> **AI-assisted project**
> This codebase was built with [Claude Code](https://claude.com/claude-code). It works for the author's specific setup but has not been independently audited. Review the code before running it in any security-sensitive or production environment.

A terminal UI for [Incus](https://linuxcontainers.org/incus/), the system
container/VM manager (LXD fork).

This is a port of [lazydocker](https://github.com/jesseduffield/lazydocker)
by [Jesse Duffield](https://github.com/jesseduffield) — the excellent Docker
TUI this project's UI, keybindings, and overall structure are lifted from.
Full credit for the original design and the [gocui](https://github.com/jesseduffield/gocui)
framework it's built on goes to Jesse and the lazydocker contributors. See
[LICENSE](LICENSE) (MIT, same as upstream), [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)
for third-party attributions, and [docs/Port.md](docs/Port.md) for what was
carried over and how it was renamed.

## Status: MVP

This is an early port covering a single panel. Feature parity with
lazydocker (images/networks/volumes/services panels, custom commands,
non-English translations) is not there yet — see "What's not here yet"
below, and [BACKLOG.md](BACKLOG.md) for the panel-by-panel comparison.

## Requirements

- An [Incus](https://linuxcontainers.org/incus/) daemon (`incusd`) running
  locally, reachable via its unix socket.
- Your user in the `incus`/`incus-admin` group (or root) for socket access.
- The `incus` CLI on `PATH` — used for the exec-into-instance feature.
- Go 1.27+ to build from source.

## Install / run

```sh
git clone https://github.com/tallica/lazyincus.git
cd lazyincus
go build -o lazyincus .
./lazyincus
```

or run directly without building a binary:

```sh
go run .
# or
make run
```

Flags: `-d` / `--debug` for debug logging, `--version` to print version info.

## Usage

Five side panels: **Instances** (`1`), listing both containers and VMs,
**Snapshots** (`2`) for whichever instance is selected, **Images** (`3`),
**Volumes** (`4`) and **Networks** (`5`). All are scoped to one Incus project
at a time (`P` switches project; the footer shows the current remote and
project).

The instances panel's columns (name, status, type, IPv4, IPv6, snapshot
count) can be reordered or hidden via `gui.instanceColumns` in the config
file - see [docs/Config.md](docs/Config.md).

| Key | Action |
|---|---|
| `1` … `5` | Focus the Instances / Snapshots / Images / Volumes / Networks panel |
| `tab` / `shift+tab` | Next / previous side panel |
| `↑`/`↓`, `j`/`k` | Navigate |
| `PgUp`/`PgDn`, `J`/`K`, `H`/`L` | Scroll the main panel |
| `enter` | Focus main panel (Stats / Logs / Config / Env / Top tabs) |
| `[` / `]` | Switch main-panel tab |
| `S` | Start |
| `s` | Stop |
| `p` | Pause/freeze (toggle) |
| `d` | Delete the selected item (instances offer to stop first if running; only custom volumes and managed networks can be deleted) |
| `e` | Toggle showing stopped instances |
| `m` | Jump to Logs tab |
| `n` | New snapshot (snapshots panel) |
| `r` | Restart an instance, or restore a snapshot |
| `E` | Exec a shell into the instance |
| `y` | Copy the instance's IPv4 address to the clipboard |
| `P` | Switch Incus project (re-scopes the instance list) |
| `o` | Open the lazyincus config file |
| `O` | Edit the lazyincus config file in `$VISUAL`/`$EDITOR` |
| `+` / `_` | Next / previous screen mode |
| `/` | Filter |
| `x` / `?` | Keybinding menu |
| `q` | Quit |

Config file: `~/.config/lazyincus/config.yml`. See [docs/Config.md](docs/Config.md)
for the full list of options and defaults.

## What's not here yet

Ported deliberately as an MVP, not full parity with lazydocker. Not included:

- Services/Project panels (lazydocker's docker-compose view). The Incus
  analog would be [incus-compose](https://github.com/lxc/incus-compose),
  whose stacks are visible today as Incus projects — see
  [BACKLOG.md](BACKLOG.md#incus-compose-integration)
- Custom and bulk commands
- Historical resource-usage graphing (the Stats tab is point-in-time only)
- Non-English translations
- Windows support

VM instances are listed and can be started/stopped/deleted like containers,
but freeze/unfreeze and exec are untested against a real VM — see
[BACKLOG.md](BACKLOG.md#blocked) for why and what's needed to verify them.

See [CLAUDE.md](CLAUDE.md) for the architecture and the Incus API
integration details.

## Changelog

See [CHANGELOG.md](CHANGELOG.md). Unscheduled ideas and follow-up work are in [BACKLOG.md](BACKLOG.md).

## Contributing

Standard Go project layout, no vendor directory. Build/test with:

```sh
go build ./...
go vet ./...
go test ./...
```

or via the Makefile (`make build`, `make vet`, `make test`, `make lint`,
`make run`, `make clean`).

## License

MIT — see [LICENSE](LICENSE). This project carries forward the original
lazydocker copyright notice as required by its MIT license, since much of
the code here is ported/adapted from it. Third-party dependency licenses
(gocui, the Incus client library, etc.) are listed in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
