# lazyincus

A terminal UI for [Incus](https://linuxcontainers.org/incus/) (the system
container/VM manager, LXD fork), ported from
[lazydocker](https://github.com/jesseduffield/lazydocker) — a terminal UI for
Docker built on [gocui](https://github.com/jesseduffield/gocui).

This file is the orientation: how the app is put together, and how to
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
against Incus 7.4 in an Alpine VM, which is new enough for incus-compose,
so the Services panel's verbs run there for real. What that host can't
provide is a nested VM instance, leaving the VM-specific paths —
freeze/unfreeze, exec — unverified. See BACKLOG.md's Blocked section.

- Module: `github.com/tallica/lazyincus`
- Go: 1.27
- Incus client: `github.com/lxc/incus/v7` (client at `v7.3.0`)
- TUI: `github.com/jesseduffield/gocui` (pinned to
  `v0.3.1-0.20240418080333-8cd33929c513` — the version lazydocker itself
  uses; the latest tagged `v0.3.0` release is missing the `Tabs`/`TabIndex`
  view fields the main-panel tab UI depends on)

## Source of the port

Ported from a clone of lazydocker at commit
[`7e7aadc2071d58031bf2daafca1fbd4093efc23f`](https://github.com/jesseduffield/lazydocker/commit/7e7aadc2071d58031bf2daafca1fbd4093efc23f)
(2026-04-19). This repo intentionally has no shared git history with
lazydocker — it's a rewrite targeting a different backend, not a fork, so
grafting histories would imply a closer lineage than actually exists. The
commit pin is the substitute: the way to check later whether a bugfix that
landed in lazydocker's shared TUI/config plumbing after this date also
applies here.

## Side panels

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
splits evenly between whichever panels aren't hidden, or — with `gui.expandFocusedSidePanel` — gives the
focused one everything the others don't need. "Focused" there means the last
side panel to have focus, so stepping into the main panel doesn't collapse
the list you were reading.

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
  compose file in the working directory, and then it's `[1]`.
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
go build ./...
go vet ./...
go test ./...
```

No `vendor/` directory — plain module mode.

### Verifying in a live TUI

Unit tests cover almost none of the UI, so changes worth checking get driven
against a real daemon:

```sh
tmux new-session -d -s lzi -x 140 -y 40 "CONFIG_DIR=/tmp/lzi-cfg ./lazyincus"
tmux send-keys -t lzi P        # drive it
tmux capture-pane -p -t lzi    # read the rendered screen
tmux kill-session -t lzi
```

`script -q /dev/null` is not an alternative: with stdout redirected the pty
is 0x0 and gocui renders nothing. `CONFIG_DIR` points at a throwaway config
so experiments don't touch the real one.

For daemon-side fixtures, note that a fresh `incus project create` has no
root disk or NIC in its default profile — `incus launch` into it fails with
`No root device could be found` until both are added
(`incus profile device add default root disk path=/ pool=<pool> --project
<name>`, same for a `nic`). Deleting the project afterwards also needs its
cached images deleted first.
