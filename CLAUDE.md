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
verbs are exercised for real, and with a VM alongside the containers.

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
  would notice goes under `## [Unreleased]` in the same breath as the code:
  one line each, leading with what the user sees or does differently,
  usually under 25 words. Go longer only for a caveat that would otherwise
  look like a bug, or a key that moved; how it works belongs in the docs
  and the commit message. Entries before 0.10.0 are longer - don't take
  their voice from those.
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
- **Anything that changes a daemon honours read-only.** A key binding
  that starts, stops, deletes, edits, snapshots or shells into anything
  sets `Mutates: true`, and `guardReadOnly` (`pkg/gui/read_only.go`)
  refuses it on a [read-only remote](docs/Config.md#top-level) - the
  remote it acts on, which `actionRemote` works out per view. A change
  that reaches a daemon some other way than a key binding needs the same
  check by hand. When unsure, mark it: a refusal costs a keypress, a
  missed one costs production.
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
styling (including the `[n]` title prefix), the number keys, the layout
and `allSidePanels()` all derive from it, so a new panel is one entry
there plus its own `*_panel.go`, presentation and refresh loop, and one in
`paletteSources()` (`pkg/gui/command_palette.go`): without it the command
palette lists none of its items, and its actions name nothing and act on
whatever is selected when run. The palette's rows are the key bindings, so
a panel's key that doesn't act on the selected row sets `NoSelection:
true`: the palette lists it while the panel is empty and runs it without
checking the row is still the one it named. A key that acts on the row
never sets it - that check is all that keeps a refresh from turning a
restart onto the next row. A `window` on the def puts a panel in a slot it
shares with others, as one of its tabs, which is how Images, Volumes,
Networks and Profiles become the one Resources panel, and Backups a tab
beside Snapshots; the number keys, `tab` and the layout count those slots,
not views (`pkg/gui/window.go`). Order is both the top-to-bottom layout
order and the number-key order. A panel can be absent (`hidden` on the
def) - for the session, or Backups while no stack is on the session's
remote; everything user-facing is numbered over `visibleSidePanelDefs()`,
so the first *visible* panel is `[1]` and is what the app focuses at
startup - unless it's Stacks with no stack on the session's remote, which
collapses it and Services to their titles and hands the focus to Instances
([docs/Panels.md](docs/Panels.md#stacks)). `tab`/`shift+tab` cycle through
them in that order, stepping from the last side panel to have focus;
`←`/`→` and `h`/`l` do the same list by list, a shared slot's lists each a
stop. The side column splits evenly between whichever panels aren't hidden
or collapsed, or — with `gui.State.ExpandSidePanel`, seeded from config
and toggled by `=` — gives the focused one everything the others don't
need. "Focused" there means the last side panel to have focus, so stepping
into the main panel doesn't collapse the list you were reading.

Views, panels and `gui.State` belong to gocui's main loop. A refresh is a
`fetch` (`pkg/gui/refresh.go`): it asks the daemon off the loop and returns
the closure that shows the answer, which `gui.refresh` runs on the loop
through `Update`. So a refresh is never started on the loop — a keypress
wanting one starts it from `WithWaitingStatus` or `refreshInBackground` —
and `RerenderList` is never called off it. Each kind of fetch carries a
`refreshSeq`, so an older fetch that finishes late can't undo a newer one.
A refresh of several kinds applies each that succeeded even when another
fails. Refreshes come from the pollers, from actions, and from the daemon's
event stream (`pkg/gui/events.go`), which says straight away when something
changes; see [docs/Incus.md](docs/Incus.md). A stack on another remote is
the one exception to asking inside the fetch: that remote is connected to
and read in the background (`pkg/gui/remotes.go`), the fetch taking the
last answer, so a server that's away holds up no poll.

A main-panel tab's content is a string built off the main loop — on a
ticker, or in a task goroutine — so anything that needs the panel's width
reads `gui.mainViewWidth`, which `layout` stores on every pass. The view's
own `Size()` is the main loop's to read. `sectionHeading` is what wants it:
the rule it draws runs to the panel's edge.

Each panel's own behaviour — what it lists, its columns, its main-panel
tabs, what its keys do — is in [docs/Panels.md](docs/Panels.md), in this
order:

- **Stacks** — compose project directories: the working directory's, or
  `-P`'s, and every one added with `a`, saved in `state.yml`, each pinned
  to the Incus remote it was added for. Present only when `incus-compose`
  is on `PATH`, and then it's `[1]`. The only list that spans remotes:
  everything else is the session's, which `R` or `space` on a stack
  switches.
- **Services** — the selected stack's services, one row each, with a
  replicated service's instances under it; there whenever Stacks is. A
  service is its instances, so the two panels share statuses, columns and
  renderers; only `partial`, `none` and the replica count are the
  service's own.
- **Instances** — containers and VMs across every project, minus the
  instances of the services the listed stacks on the same remote declare,
  until `C` puts them back.
- **Snapshots** — follows whichever instance or service the lists above
  it have selected, or the custom volume the volumes list has, rather than
  having a selection of its own; or lists every instance's (`e`).
- **Backups** — the selected stack's `incus-compose backup` runs, a tab
  beside Snapshots on a remote with a stack listed.
- **Resources** — how Images, Volumes, Networks and Profiles share one
  slot.
- **Images**, **Volumes**, **Networks** — local images and what uses them;
  every pool's volumes in one list, with sizes; managed networks, the
  host's own interfaces behind `e`, with their leases.
- **Profiles** — every project's, with the devices each hands out.
- **Operations and warnings** — not panels but `W`'s popup: the daemon's
  operations, under way and the session's history of them, which is
  lazyincus's own record, and its warnings.

## Talking to Incus

[docs/Incus.md](docs/Incus.md) has the non-obvious parts, established
against a live daemon or read out of the Incus source rather than inferred:
how the client resolves a remote, and connects to another by name or
moves onto it in place, why a daemon going away mid-session is
never fatal, per-project clients and why item identity includes the
project, the console log's drain-on-read behaviour, why `exec` and
`attach` shell out to the `incus` CLI instead of using the client's
websocket API, the status the daemon won't give an instance mid-action,
the event stream: which events count, and the order they arrive in, and
how briefly the daemon keeps an operation once it has ended, and the
warnings no event announces.

## Config

`pkg/config/app_config.go` holds the schema, and `app_state.go` the app's
own `state.yml` beside it; [docs/Config.md](docs/Config.md) documents both
and the files' location precedence.

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
`go test ./pkg/gui -update`; read the diff before keeping it. CI runs the
tests under `-race` (`go test -race ./...`), which `make test` doesn't:
run that too when a change touches timing.

### Releasing

CI is `.github/workflows/ci.yml`: test, lint and govulncheck on every push
to master, pull request and `v*` tag. govulncheck lives in `vuln.yml`,
which CI calls and which also runs weekly on its own. On a tag its
`release` job waits for all three and publishes nothing unless they pass.
Then GoReleaser builds macOS and Linux binaries for amd64 and arm64,
archives each with the markdown and LICENSE, and publishes them with a
`checksums.txt`. The release notes are the tag's `CHANGELOG.md` section
(`scripts/release-notes.sh`), so a tag without one fails the release
rather than publishing an empty one. `make release-snapshot` builds the
same set into `dist/` without touching GitHub, and `make release-check`
validates `.goreleaser.yaml`.

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
event stream against a real daemon, the main-panel tasks, the loop they
hand results to. A change that touches any of them gets a race-enabled
build driven like the one above:

```sh
go build -race -o /tmp/lazyincus-race .
tmux new-session -d -s lzr -x 140 -y 40 \
  "GORACE=log_path=/tmp/lzi-race CONFIG_DIR=/tmp/lzi-cfg /tmp/lazyincus-race"
```

Move through the lists, every main-panel tab, `W`'s popup, a project
switch or two (`P`), and the stacks and services panels if incus-compose
is installed, for a minute or so; then quit. With a second remote to hand,
list a stack on it and switch to it and back (`R`, `space`): connecting
and reading it are goroutines of their own. Any `/tmp/lzi-race.*` file is
a race. Read-only keys are enough - the races are between reading and
refreshing. For the event stream, change something from a shell meanwhile
- create and delete a throwaway profile - so events arrive while you move.
