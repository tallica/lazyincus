# lazyincus

> [!WARNING]
> **AI-assisted project**
> This codebase was built with [Claude Code](https://claude.com/claude-code). It works for the author's specific setup but has not been independently audited. Review the code before running it in any security-sensitive or production environment.

A terminal UI for [Incus](https://linuxcontainers.org/incus/) — a
next-generation system container, application container, and virtual
machine manager.

This is a port of [lazydocker](https://github.com/jesseduffield/lazydocker)
by [Jesse Duffield](https://github.com/jesseduffield) — the excellent Docker
TUI this project's UI, keybindings, and overall structure are lifted from.
Full credit for the original design goes to Jesse and the lazydocker
contributors. The framework underneath is [gocui](https://github.com/jroimartin/gocui)
by Roi Martin, used here through [Jesse's fork](https://github.com/jesseduffield/gocui).
See [LICENSE](LICENSE) (MIT, same as upstream),
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for third-party
attributions, and [docs/Port.md](docs/Port.md) for what was carried over and
how it was renamed.

![lazyincus](docs/screenshot.png)

## Status

Past the MVP it started as, and in daily use — still pre-1.0. Panels for
instances, snapshots, images, volumes, networks and — in a directory with a
compose file — services, plus the Incus concepts lazydocker has no analog
for: projects and remotes. Parity with lazydocker in the corners —
custom and bulk commands, non-English translations — isn't there; see
"What's not here yet" below, and [BACKLOG.md](BACKLOG.md) for the
panel-by-panel comparison.

## Requirements

- An [Incus](https://linuxcontainers.org/incus/) daemon (`incusd`) to talk to —
  local over its unix socket, or any remote your `incus` CLI is configured for
  (see [docs/Remotes.md](docs/Remotes.md)).
- Your user in the `incus`/`incus-admin` group (or root) for local socket
  access.
- The `incus` CLI on `PATH` — used for attaching to a console and for
  exec-into-instance.
- Go 1.27+ to build from source.
- Optional: [incus-compose](https://incus-compose.org), if you run compose
  stacks on Incus. It's what the optional `service`, `health` and `image`
  columns describe ([docs/Config.md](docs/Config.md)); those columns read
  the daemon, so a stack shows up in them from another machine entirely,
  while the Services panel needs the `incus-compose` binary on `PATH` and
  the compose file in the current directory. Not in Homebrew core — on
  macOS, `brew install tallica/tap/incus-compose`.

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

## Remotes

lazyincus talks to whichever remote the `incus` CLI treats as its default.
Point it at a different one with:

```sh
INCUS_REMOTE=myserver lazyincus
```

The footer shows the remote you are on. [docs/Remotes.md](docs/Remotes.md)
covers adding a remote and its token, reaching a daemon over SSH, and the
gotchas behind running incusd in a local VM — macOS Local Network Privacy and
guest clock skew both fail in ways that point at the wrong component.

## Usage

Five side panels: **Instances** (`1`), listing both containers and VMs,
**Snapshots** (`2`) for whichever instance is selected, **Images** (`3`),
**Volumes** (`4`) and **Networks** (`5`). All list every Incus project by
default; `P` scopes them to a single project instead, and the footer shows
the current remote and scope. A project column appears on any panel whose
contents actually span projects, and actions run against the project the
item came from.

Start lazyincus from a directory holding an
[incus-compose](https://incus-compose.org) file and a sixth panel appears
above the others: **Services** (`1`), titled with the compose project, one
row per service that file declares, with the other panels shifting down a
number. A service the file declares but nothing is running still gets a
row, in state `none` — the
compose file is what the list comes from, not the daemon. The stack's own
instances move out of the instances panel, which becomes **Standalone
Instances** (`2`); every other project's instances stay there. "Directory"
follows incus-compose's own resolution, so `INCUS_COMPOSE_PROJECT_DIRECTORY`
in the environment works too, not just the current directory (lazyincus
shells out to incus-compose without passing its own `-P`, so the flag itself
isn't reachable this way).

The instances panel's columns (name, status, health, type, IPv4, snapshot
count) can be reordered or hidden via `gui.instanceColumns` in the config
file, and the Services panel's (name, status, health, IPv4, snapshot count) via
`gui.serviceColumns` - see [docs/Config.md](docs/Config.md). Health comes
from `ic-healthd`, so it's blank for anything created outside
incus-compose.

| Key | Action |
|---|---|
| `1` … `6` | Focus a side panel, numbered top to bottom as shown in its title |
| `tab` / `shift+tab` | Next / previous side panel |
| `↑`/`↓`, `j`/`k` | Navigate |
| `PgUp`/`PgDn`, `J`/`K`, `H`/`L`, `h`/`l` | Scroll the main panel |
| `enter` | Focus main panel (Info / Logs / Config / Env / Top tabs) |
| `[` / `]` | Switch main-panel tab |
| `S` | Start; on the Services panel, `incus-compose start` for the selected service |
| `s` | Stop; on the Services panel, `incus-compose stop` for the selected service (confirms first) |
| `p` | Pause/freeze (toggle) |
| `d` | Delete the selected item (instances offer to stop first if running; only custom volumes and managed networks can be deleted); on the Services panel, bring the service down (menu: plain or with volumes) |
| `u` | Services panel: bring the service up (`incus-compose up --detach <service>`) |
| `U` | Services panel: pull the latest image and recreate the service's instances (confirms first) |
| `e` | Toggle showing stopped instances |
| `m` | Jump to Logs tab |
| `n` | New snapshot of the selected instance, from either panel — name it, `tab` to the expiry/stateful fields, `enter` or `ctrl+s` to create |
| `r` | Restart an instance, or restore a snapshot; on the Services panel, `incus-compose restart` for the selected service |
| `a` | Attach to the instance's console (`incus console`) |
| `E` | Exec a shell into the instance |
| `C` | Services panel: the compose verbs without a key of their own — kill, pause, unpause, build, pull, `logs --follow` — each for the selected service and for the whole project |
| `y` | Copy the instance's IPv4 address to the clipboard |
| `P` | Switch Incus project (re-scopes the instance list) |
| `o` | Open the lazyincus config file |
| `O` | Edit the lazyincus config file in `$VISUAL`/`$EDITOR` |
| `+` / `_` | Next / previous screen mode |
| `/` | Filter |
| `x` / `?` | Keybinding menu |
| `q` | Quit |

On the Services panel the per-instance keys - `m`, `n`, `a`, `E`, `y` - act
on the service's instance, asking which one when it has replicas.

Config file: `~/.config/lazyincus/config.yml`. See [docs/Config.md](docs/Config.md)
for the full list of options and defaults.

## Supporting upstream

lazyincus is a thin layer over other people's work — Incus does everything
that matters here, and lazydocker is what it was ported from. Neither asks
anything of you for that, but the people behind them take sponsorships:

- [Stéphane Graber](https://github.com/sponsors/stgraber) — project leader
  of [Linux Containers](https://linuxcontainers.org/), the umbrella over
  Incus, IncusOS, LXC and more.
- [Jesse Duffield](https://github.com/sponsors/jesseduffield) — author of
  lazydocker, lazygit and more.

If lazyincus is useful to you, that money is better spent there than here.

## What's not here yet

Not full parity with lazydocker. Not included:

- The credits and aggregate-log tabs lazydocker's Services/Project panel
  had — the panel itself and the compose verbs are covered by the Services
  panel above; see
  [BACKLOG.md](BACKLOG.md#incus-compose-integration) for what's left
- Custom and bulk commands
- Historical resource-usage graphing (the Info tab's stats are point-in-time only)
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
