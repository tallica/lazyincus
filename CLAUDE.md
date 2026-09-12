# lazyincus

A terminal UI for [Incus](https://linuxcontainers.org/incus/) (the system
container/VM manager, LXD fork), ported from
[lazydocker](https://github.com/jesseduffield/lazydocker) — a terminal UI for
Docker built on [gocui](https://github.com/jesseduffield/gocui).

This file documents the current architecture and the Incus-specific
behaviors behind it. What shipped when is in [CHANGELOG.md](CHANGELOG.md);
what hasn't been built and why (including the panel-by-panel comparison
against lazydocker) is in [BACKLOG.md](BACKLOG.md); the rename map and
package layout are in [docs/Port.md](docs/Port.md); the config schema is in
[docs/Config.md](docs/Config.md).

## Status

MVP, run end-to-end against a real daemon (via `colima start --runtime
incus` on macOS). VM-specific paths remain unverified — see BACKLOG.md's
Blocked section.

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
top-to-bottom layout order and the number-key order; the first panel is what
the app focuses at startup; `tab`/`shift+tab` cycle through them in that
order, stepping from the last side panel to have focus. The side column splits evenly between whichever
panels aren't hidden, or — with `gui.expandFocusedSidePanel` — gives the
focused one everything the others don't need. "Focused" there means the last
side panel to have focus, so stepping into the main panel doesn't collapse
the list you were reading.

### Instances

Lists containers and VMs
(`GetInstances(api.InstanceTypeAny)`) scoped to one Incus project at a time
(`P` switches). Columns mirror `incus list`: name, status, type, IPv4, IPv6,
snapshot count. Type, addresses and snapshot count only appear once
`RefreshInstanceDetails` has fetched full details in the background.

Rows sort by name, with stopped instances last (`sortInstances`), and the
cursor follows the selected item across a re-sort rather than holding its
index — otherwise stopping an instance moves it down the list and hands the
selection to whatever took its place.

Which columns show, and in what order, is user-configurable via
`gui.instanceColumns`. `presentation.GetInstanceDisplayStrings` looks up
each configured name in the `instanceColumnRenderers` map
(`pkg/gui/presentation/instances.go`) and skips anything unrecognized, so a
new column means one entry in that map plus the default/valid-values list in
`pkg/config/app_config.go`.

Keybindings live in [README.md](README.md#usage) — the canonical source,
keep that table current rather than duplicating it here. Two behaviors it
doesn't convey: `s`/`d` confirm before acting, and `p` toggles between
`freeze` and `unfreeze` depending on current status.

Main panel tabs, roughly what `incus info <name>` prints in one shot, split
up:

- **Stats** — CPU/memory/process count/disk/network from
  `InstanceFull.State`, re-rendered every second (no extra API calls: the
  background poll already keeps that current). CPU is cumulative usage
  time, not a percentage — the API reports total nanoseconds since start,
  not a rate, and `incus info` shows the same.
- **Logs** — polls `Instance.TailConsoleLog()` and re-renders the
  accumulated buffer. See "Logs" below for why a raw snapshot doesn't work.
- **Config** — YAML dump of `api.InstanceFull`.
- **Env** — `environment.*` entries from `ExpandedConfig` (expanded, so
  profile-inherited variables show up too).
- **Top** — process list, polled every two seconds. Incus's API reports a
  process *count* and nothing more, so this execs `ps` inside the instance
  over the client's websocket exec (no terminal needed, unlike the `E`
  shell-out). Images without a `ps` and VMs without the guest agent get the
  daemon's error instead of a list; the flag set that works is cached per
  instance, since busybox and util-linux `ps` disagree on all of them.

### Snapshots

Follows the instances panel: its `OnSelect` calls `refreshSnapshots`, so the
panel always shows the selected instance, and the view title carries that
instance's name since the rows alone don't say whose they are. Listing uses
`GetInstanceSnapshots` rather than the `InstanceFull.Snapshots` the
background poll already holds, because create and delete have to show up
immediately. Snapshot names come back from the API prefixed with the
instance (`alpine/snap0`); every other call wants the bare name, which
`snapshotName` strips. Restore is an instance update carrying `Restore:
<name>`, not a snapshot operation.

### Images

Local images (`GetImages`), identified by `Image.Label()`: alias, else the
description an unaliased cached image carries (what `incus image list` shows
in its DESCRIPTION column), else the short fingerprint. Truncated to keep
the columns after it on screen. `d` deletes after a confirmation. Polled every 10s rather than the instance list's 2s: images
only change when someone pulls or deletes one.

### Volumes

Every storage pool's volumes in one list (`GetVolumes` walks
`GetStoragePoolNames` then `GetStoragePoolVolumes` per pool; a pool that
errors is skipped rather than emptying the panel). Identity is
pool+type+name, since an instance and a custom volume can share a name.
Only `custom` volumes can be deleted — the rest go away with the instance or
image they belong to.

### Networks

`GetNetworks`, managed and unmanaged alike. Only managed ones can be
deleted; the unmanaged entries are host interfaces Incus merely reports.

## Incus client integration details

The non-obvious parts of talking to Incus. Most of these were established
against a live daemon or read out of the Incus source, not inferred.

- **Connection**: `cliconfig.LoadConfig("")` +
  `cliCfg.GetInstanceServer(cliCfg.DefaultRemote)` in
  `pkg/commands/incus.go`, reusing Incus's own remote resolution rather than
  a bare socket connect. It reads `~/.config/incus/config.yml` (or
  `$INCUS_CONF`) and handles unix-socket and TLS remotes alike. This matters
  beyond Linux hosts: under `colima start --runtime incus` the "local"
  socket lives at a path recorded in that remote's config
  (`unix:///Users/you/.colima/default/incus.sock`), not at any standard
  Linux location.
- **Projects**: Incus scopes instances (and networks, volumes, profiles) per
  project; the client is scoped to one at a time, `default` unless the
  remote says otherwise. `UseProject` swaps `IncusCommand.client` under
  `clientMutex`, hence the `Client()` accessor rather than a field, and
  `GetInstances` reassigns `Instance.Client` on every refresh. `P`
  (`handleSwitchProject` in `pkg/gui/projects.go`) switches.
- **All projects**: "all projects" in that menu lists every project at once
  through the `*AllProjects` endpoints. Each item then carries its own
  project-scoped client (`clientFor`), so actions go to the project the item
  came from - including `RefreshInstanceDetails`, which asks each instance's
  client rather than the command's, and the `incus` CLI shell-outs, which
  pass `--project`. Identity includes the project everywhere items are
  matched across refreshes: two projects can hold an instance, image or
  volume of the same name.
- **List instances**: `GetInstances(api.InstanceTypeAny)` returns
  `[]api.Instance`. Existing `*Instance` objects are matched by name and
  reused across refreshes so cached `full` details survive.
- **Full details / state**: `GetInstanceFull(name)` → `*api.InstanceFull`
  (embeds `api.Instance` + `*api.InstanceState`). Fetched per instance in
  the background every second (`RefreshInstanceDetails`), not on every list
  refresh.
- **State changes**: `UpdateInstanceState(name, api.InstanceStatePut{Action:
  ...}, "")` then `op.Wait()`. Stop/restart use a 30s timeout;
  start/freeze/unfreeze use -1.
- **Delete**: Incus refuses to delete a running instance with a plain 400
  whose body is the string `Instance is running` (`instanceDelete` in
  `cmd/incusd/instance_delete.go`) — no dedicated error code, so
  `asDeleteError` matches that message and converts it to the
  `ErrInstanceRunning` sentinel, and the panel offers force-stop-then-delete
  (`Instance.ForceDelete`, mirroring `incus delete --force`: stop with
  `Force: true`, then delete, skipping the delete for ephemeral instances
  that Incus discards on stop). The message match can't be replaced with an
  `IsRunning()` pre-check: the daemon counts a *frozen* instance as running,
  ours only matches status `Running`.
- **Logs**: `GetInstanceConsoleLog` returns an `io.ReadCloser` over the
  console ring buffer — pull-based, not a stream like Docker's
  `ContainerLogs(..., Follow: true)`, and **drain-on-read**: each read
  returns only what was buffered since the previous one (confirmed against
  the `incus` CLI itself — `incus console <name> --show-log` twice in a row
  shows output then nothing). `TailConsoleLog` therefore accumulates reads
  into a capped 256 KiB per-instance buffer, and the tab polls that.
  Drain-on-read only holds while the instance runs; once stopped, incusd
  serves the persisted log file in full on every request
  (`instanceConsoleLogGet`), so `TailConsoleLog` checks `IsRunning()` and
  fetches just once after a stop — otherwise every poll re-appends the whole
  log. There's no header-based staleness check available: the client returns
  only `resp.Body` and discards `Last-Modified`.
- **Attach**: `a` shells out to `incus console <name>`, the analog of
  lazydocker's `docker attach`. No detach hint from us - the CLI prints its
  own (`ctrl+a q`) on connect.
- **Exec**: deliberately not the client library's `ExecInstance` websocket
  API — that needs the session's stdio wired into the terminal, which is
  nontrivial to thread through gocui's suspend/resume model (raw mode,
  window-resize messages). `instanceExecShell` shells out to the `incus` CLI
  instead, the same pattern lazydocker uses for `docker exec`. Trade-off:
  requires the `incus` binary on PATH, not just socket access.

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
