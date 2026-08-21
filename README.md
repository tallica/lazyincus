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
for third-party attributions, and [CLAUDE.md](CLAUDE.md) for a detailed
line-by-line account of what was carried over, adapted, or dropped.

## Status: MVP

This is an early port covering a single panel. Feature parity with
lazydocker (images/networks/volumes/services panels, custom commands,
stats, non-English translations) is not there yet — see "What's not here
yet" below. See [CLAUDE.md](CLAUDE.md#status) for current build/test status
and what has and hasn't been verified against a real Incus daemon.

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

The app currently shows one panel: **Instances**, listing both containers
and VMs.

| Key | Action |
|---|---|
| `↑`/`↓`, `j`/`k` | Navigate |
| `enter` | Focus main panel (Stats / Logs / Config / Snapshots tabs) |
| `[` / `]` | Switch main-panel tab |
| `S` | Start |
| `s` | Stop |
| `r` | Restart |
| `p` | Pause/freeze (toggle) |
| `d` | Delete |
| `e` | Toggle showing stopped instances |
| `m` | Jump to Logs tab |
| `E` | Exec a shell into the instance |
| `/` | Filter |
| `x` / `?` | Keybinding menu |
| `q` | Quit |

Config file: `~/.config/lazyincus/config.yml`. See [docs/Config.md](docs/Config.md)
for the full list of options and defaults.

## What's not here yet

Ported deliberately as an MVP, not full parity with lazydocker. Not included:

- Images, Networks, Volumes panels
- Services/Project panels (lazydocker's docker-compose view — Incus has no
  direct equivalent)
- Custom and bulk commands
- Top tab (per-instance process list) and historical resource-usage graphing
- Non-English translations
- Windows support

See [CLAUDE.md](CLAUDE.md) for the full rationale, the Incus API integration
details, and known unverified assumptions (this was built without access to
a live Incus daemon).

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

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
