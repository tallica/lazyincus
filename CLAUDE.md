# lazyincus

A terminal UI for [Incus](https://linuxcontainers.org/incus/) (the system
container/VM manager, LXD fork), ported from
[lazydocker](https://github.com/jesseduffield/lazydocker) — a terminal UI for
Docker built on [gocui](https://github.com/jesseduffield/gocui).

[README.md](README.md) is the front door for people: what lazyincus is, how
to install it, and every key it binds. This file is for whoever is working
in the tree — the rules to work by, how the app is put together, and how to
build, run and verify a change. The detail sits alongside it — each panel's
own behaviour in [docs/Panels.md](docs/Panels.md), talking to the daemon in
[docs/Incus.md](docs/Incus.md), the rename map and package layout in
[docs/Port.md](docs/Port.md), the config schema in
[docs/Config.md](docs/Config.md), and connecting to a non-default remote,
with the gotchas behind a daemon in a local VM, in
[docs/Remotes.md](docs/Remotes.md). What shipped when is in
[CHANGELOG.md](CHANGELOG.md); what hasn't been built and why — including
the panel-by-panel comparison against lazydocker — is in
[BACKLOG.md](BACKLOG.md).

## Status

Past the MVP it started as and in daily use; still pre-1.0. Developed
against Incus 7.4, new enough for incus-compose, so the Services panel's
verbs are exercised for real. VM instances aren't: nothing to hand can nest
one, leaving the VM-specific paths — freeze/unfreeze, exec — unverified.
See [BACKLOG.md](BACKLOG.md#blocked).

- Module: `github.com/tallica/lazyincus`
- Go: 1.27
- Incus client: `github.com/lxc/incus/v7` (client at `v7.5.1`)
- TUI: `github.com/jesseduffield/gocui` (pinned to master,
  `v0.3.1-0.20260331125330-c81715e95462` — the latest tagged `v0.3.0`
  release is missing the `Tabs`/`TabIndex` view fields the main-panel tab
  UI depends on. lazydocker is still on `8cd33929c513` from 2024, so for
  how a newer gocui API is meant to be used, lazygit is the reference: it
  tracks gocui master.)

## Working on this

- **Committing is the maintainer's call.** Make the change, run the checks,
  say what happened and what you couldn't verify. A commit or a push waits
  to be asked for; the diff gets read first.
- **`make lint` belongs with build, vet and test.** `.golangci.yml` is
  stricter than `go vet`, and a lint failure is no better found later.
- **CHANGELOG.md is part of the change, not a follow-up.** Anything a user
  would notice goes under `## [Unreleased]` in the same breath as the code,
  in the voice the released entries use: what changed and why it's better,
  not which functions moved.
- **One home per fact.** The map above says which file owns what. Link to
  the home instead of restating it — a fact in two places is a fact that
  will go stale in one of them.
- **Comments earn their line.** One line, unless it carries what the code
  can't say: a daemon quirk, an upstream constraint, why the obvious
  approach doesn't work. Before committing, re-read every comment the
  change added and cut the ones that only say again what the code says.
- **Nothing machine-specific in the tree.** The repo is public and a push
  is permanent — no credentials, no local paths, no "works on my VM". That
  belongs in what you report, not in a file.
- **The tests see the screen, not the daemon.** The screen and refresh
  tests in `pkg/gui` run the real app on a headless gocui over
  `incustest`'s stand-in daemon; anything that talks to a real one still
  wants driving live — see [Verifying in a live TUI](#verifying-in-a-live-tui).

## Source of the port

Ported from a clone of lazydocker at commit
[`7e7aadc2071d58031bf2daafca1fbd4093efc23f`](https://github.com/jesseduffield/lazydocker/commit/7e7aadc2071d58031bf2daafca1fbd4093efc23f)
(2026-04-19). This repo intentionally has no shared git history with
lazydocker — it's a rewrite targeting a different backend, not a fork, so
grafting histories would imply a closer lineage than actually exists. The
commit pin is the substitute: the way to check later whether a bugfix that
landed in lazydocker's shared TUI/config plumbing after this date also
applies here.

## Panels

`sidePanelDefs()` in `pkg/gui/side_panels.go` is the ordered list of side
panels — view name, title, view pointer, panel accessor. View creation,
styling (including the `[n]` title prefix), the number keys, the layout and
`allSidePanels()` all derive from it, so a new panel is one entry there plus
its own `*_panel.go`, presentation and refresh loop. Order is both the
top-to-bottom layout order and the number-key order. A panel can be absent
for the session (`hidden` on the def); everything user-facing is numbered
over `visibleSidePanelDefs()`, so the first *visible* panel is `[1]` and is
what the app focuses at startup. `tab`/`shift+tab` cycle through them in
that order, stepping from the last side panel to have focus. The side column
splits evenly between whichever panels aren't hidden, or — with
`gui.State.ExpandSidePanel`, seeded from config and toggled by `=` — gives
the focused one everything the others don't need. "Focused" there means the
last side panel to have focus, so stepping into the main panel doesn't
collapse the list you were reading.

Views, panels and `gui.State` belong to gocui's main loop. A refresh is a
`fetch` (`pkg/gui/refresh.go`): it asks the daemon off the loop and returns
the closure that shows the answer, which `gui.refresh` runs on the loop
through `Update`. So a refresh is never started on the loop — a keypress
wanting one starts it from `WithWaitingStatus` or `refreshInBackground` —
and `RerenderList` is never called off it. Each kind of fetch carries a
`refreshSeq`, so an older fetch that finishes late can't undo a newer one.
A refresh of several kinds applies each that succeeded even when another
fails.

A main-panel tab's content is a string built off the main loop — on a
ticker, or in a task goroutine — so anything that needs the panel's width
reads `gui.mainViewWidth`, which `layout` stores on every pass. The view's
own `Size()` is the main loop's to read. `sectionHeading` is what wants it:
the rule it draws runs to the panel's edge.

Each panel's own behaviour — what it lists, its columns, its main-panel
tabs, what its keys do — is in [docs/Panels.md](docs/Panels.md), in this
order:

- **Services** — a compose stack's services, one row each, with a
  replicated service's instances under it. Present only when there's a
  compose file in the working directory, and then it's `[1]`. A service is
  its instances, so the two panels share statuses, columns and renderers;
  only `partial`, `none` and the replica count are the service's own.
- **Instances** — containers and VMs across every project, minus the local
  stack's when the services panel is holding those.
- **Snapshots** — follows whichever instance the list above it has
  selected, rather than having a selection of its own.
- **Images**, **Volumes**, **Networks** — local images; every pool's
  volumes in one list; managed and unmanaged networks alike.

## Talking to Incus

[docs/Incus.md](docs/Incus.md) has the non-obvious parts, established
against a live daemon or read out of the Incus source rather than inferred:
how the client resolves a remote, why a daemon going away mid-session is
never fatal, per-project clients and why item identity includes the
project, the console log's drain-on-read behaviour, and why `exec` and
`attach` shell out to the `incus` CLI instead of using the client's
websocket API.

## Config

`pkg/config/app_config.go` holds the schema; [docs/Config.md](docs/Config.md)
documents it and the file's location precedence.

The config reloads at runtime, from `handleEditConfig` and from a
modification-time poll (`gui.configReloader`). `gui.reloadConfig` re-applies
only what the app caches — theme, view styling (`styleAllViews`, split out
of `createAllViews` for this), mouse support, columns; the rest is read at
the point of use.

## Building / testing

```sh
go build ./...   # every package
make build       # just the ./lazyincus binary, version stamped
make vet
make test
make lint        # golangci-lint, configured by .golangci.yml
make audit       # govulncheck: reachable vulnerabilities; CI runs it weekly too
```

No `vendor/` directory — plain module mode.

The screen tests compare against `pkg/gui/testdata/screens`. A change
that moves the layout on purpose rewrites them with
`go test ./pkg/gui -run TestScreen -update`; read the diff before
keeping it.

### Releasing

Pushing a `v*` tag runs `.github/workflows/release.yml`: GoReleaser builds
macOS and Linux binaries for amd64 and arm64, archives each with the
markdown and LICENSE, and publishes them with a `checksums.txt`. The
release notes are the tag's `CHANGELOG.md` section
(`scripts/release-notes.sh`), so a tag without one fails the release rather
than publishing an empty one. `make release-snapshot` builds the same set
into `dist/` without touching GitHub, and `make release-check` validates
`.goreleaser.yaml`.

A release isn't finished at the tag. GoReleaser doesn't publish to Homebrew,
so [tallica/homebrew-tap](https://github.com/tallica/homebrew-tap) still
serves the previous version until `Formula/lazyincus.rb` points at the new
archives, each `sha256` taken from the release's `checksums.txt`. The tap's
own CLAUDE.md says what else changes there with it.

The markdown that goes into an archive is a rewritten copy: away from a
checkout, a link to `docs/` or `BACKLOG.md` has nothing to resolve against,
so `scripts/absolute-links.sh` pins those links to the tag. It's the same
filter the release notes go through.

### Verifying in a live TUI

Build with `make build` and drive the binary:

```sh
tmux new-session -d -s lzi -x 140 -y 40 "CONFIG_DIR=/tmp/lzi-cfg ./lazyincus"
tmux send-keys -t lzi P        # drive it
tmux capture-pane -p -t lzi    # read the rendered screen
tmux kill-session -t lzi
```

`script -q /dev/null` is not an alternative: with stdout redirected the pty
is 0x0 and gocui renders nothing. `CONFIG_DIR` points at a throwaway config
so experiments don't touch the real one.

The cursor is not where you assume. It stays where the last keypress left
it, panels remember it across a refresh, and `capture-pane` doesn't show
which row is highlighted. Before any key that stops or deletes something,
read back what the confirmation names — that is the only thing standing
between a test and someone's running container.

For daemon-side fixtures, note that a fresh `incus project create` has no
root disk or NIC in its default profile — `incus launch` into it fails with
`No root device could be found` until both are added
(`incus profile device add default root disk path=/ pool=<pool> --project
<name>`, same for a `nic`). Deleting the project afterwards also needs its
cached images deleted first.

### Checking for races

Most of the concurrency lives in paths no test reaches: the pollers, the
main-panel tasks, the loop they hand results to. A change that touches any
of them gets a race-enabled build driven like the one above:

```sh
go build -race -o /tmp/lazyincus-race .
tmux new-session -d -s lzr -x 140 -y 40 \
  "GORACE=log_path=/tmp/lzi-race CONFIG_DIR=/tmp/lzi-cfg /tmp/lazyincus-race"
```

Move through the lists, every main-panel tab, a project switch or two
(`P`), and the services panel if there's a stack in the working directory,
for a minute or so; then quit. Any `/tmp/lzi-race.*` file is a race.
Read-only keys are enough - the races are between reading and refreshing.
