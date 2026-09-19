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
[docs/Config.md](docs/Config.md); connecting to a non-default remote, and the
gotchas behind a daemon in a local VM, are in
[docs/Remotes.md](docs/Remotes.md).

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

### Services

Only there when there's a compose file in lazyincus's own working directory,
and then it's the first side panel — the shape lazydocker takes when a
`compose.yaml` is local. `SideListPanel.Hide` is what removes it (its first
caller), and the layout already copes: `setViewFromDimensions` marks a view
with no box invisible.

That gate means panel numbering can't index `sidePanelDefs()` — a hidden
first panel would leave a hole at `[1]`. `visibleSidePanelDefs` is what the
number keys, the title prefixes, `sideViewNames` and the startup focus all
run over instead. Hidden-ness is a `hidden` func on the def rather than the
panel's own `Hide`, because views are styled and keys bound before
`setPanels` has built any panel to ask.

It also means the lookup runs before anything else: `Run` calls
`localComposeProject` at the top, ahead of `createAllViews`, since it
decides whether the panel exists at all. One `incus-compose config --format
json` shell-out, once — the working directory doesn't change mid-session.
No compose file means the command exits 1 and there's no local project; not
treated as an error, since most servers with compose stacks aren't
administered from this directory.

Rows come from that same JSON, not from the daemon: `parseComposeConfig`
reads `.name` and `.services`, so a service the compose file declares but
nothing is running still gets a row, in state `none`.
`GetComposeServices` then pairs each declared service with the project's
instances, matching on `user.label.incus-compose.service` via
`Instance.ComposeService()`. It fetches with `GetProjectInstances` rather
than reading the instances panel's list, since the panels can be scoped
anywhere. An instance whose label names no declared service — a one-off
from `incus-compose run` — matches no row and stays in the instances panel.

`ComposeService.Status` is an instance status — the daemon's own spelling,
rendered by the instances panel's own `presentation.DisplayStatus`, a
service being the instances underneath it. Only two values are the
service's own: `partial` when replicas disagree, `none` when it has no
instances. `Health` rolls up ic-healthd's verdict worst-first.

The instances panel is the other half of the split: its filter drops the
local project's compose instances, and its title becomes "Standalone
Instances". Another project's compose instances stay — they have no panel
of their own. `isLocalComposeInstance` treats an instance whose details
haven't loaded yet as the stack's, because `ComposeService()` reads
`ExpandedConfig` and would otherwise show the stack's rows for a second and
then take them away. `SpansProjects.Instances` is computed over what's left
after that filter, not over everything the daemon returned.

Because the stack has a panel of its own, startup no longer scopes the
client to it — the instances panel spans projects like every other panel,
and `P` still narrows.

Keys act on the selected service, passing its name as the `SERVICE`
argument every incus-compose verb takes; which key runs which verb is
README's [Compose stacks](README.md#compose-stacks) section, alongside the
rest of what a user sees. `composeRun` is all of them, and refreshes the
instances and services panels once the subprocess returns rather than
waiting for the poll. `s` and `d` confirm, `S`/`r`/`u` don't — same rule as
the instances panel. `U` can fail on a non-local daemon for reasons that
are incus-compose's, not ours — see [BACKLOG.md](BACKLOG.md#caveats).

The per-instance keys (`m`, `n`, `a`, `E`, `y`) reach the service's
instance through `withServiceInstance`, which acts directly on the only one
and otherwise asks which — the reason `handleSnapshotCreate` and
`handleInstanceCopyIPv4` were split into handler and action.

Columns work the way the instances panel's do: `gui.serviceColumns` over
`serviceColumnRenderers` in `pkg/gui/presentation/services.go`, taking the
instance column names rendered from the service's instances rolled up, plus
`replicas`. That one is blank unless what's running differs from what the
compose file declared — `presentation.ServiceReplicas` is the rule, and the
Info tab's line calls it too. `gui.instanceStatusStyle` covers the status column
either way; `serviceStatusStyles` supplies the glyphs for `partial` and
`none`, which no instance state has. There's no project
column: every row shares the one project, so `servicesPanelTitle` puts it
in the title instead.

Main panel tabs:

- **Info** — what the compose file declares for the service, then each
  instance's own Info tab, `instanceInfoStr` and all, which is why this
  renders on a ticker. `instanceInfoStr` takes the identity lines to leave
  out, the service having just said them: project and image always, health
  for a lone instance, and the name when the instance carries the service's
  own. The compose fields are parsed once at startup by
  `parseComposeConfig` and stored rendered, only display wanting them; the
  Healthcheck line is the project's, from `State.ComposeProject`
  (`GetComposeProject` fetches it during `refreshServices`, so rendering
  makes no API call). The image is `ComposeService.ResolvedImage`, a
  running instance's reference before the compose file's, whose own value
  may carry no registry host. Each refresh builds new `ComposeService`
  values, so the instance count is part of `GetItemContextCacheKey` —
  otherwise the ticker goes on rendering the service object the tab opened
  with.
- **Logs** — delegates to the instance logs renderer for a single-instance
  service; merging several replicas' drain-on-read buffers into one ordered
  stream is a different problem, so a replicated service points at `C`'s
  `logs --follow` instead.
- **Env** and **Top** — the instance's own, through `serviceInstanceTab`:
  the service's only instance answers for it, and replicas say so instead,
  the same split the per-instance keys make through `withServiceInstance`.
- **Config** — both halves: a Compose section, the service's slice of
  `incus-compose config --format json` handed to `yaml.JSONToYAML`
  untouched (decoding through `map[string]any` first would turn every count
  into a float64 and render `replicas: 2` as `2.0`), then one
  `instanceConfigStr` dump per instance. Those are headed `Instance`, named
  `Instance (web-1)` only when there are replicas to tell apart: a lone
  instance carries the service's own name, which under "Compose" reads as
  another view of the compose file.

The credits tab and aggregate-logs tab from lazydocker's Project panel
aren't ported — see [BACKLOG.md](BACKLOG.md#3-project-panel).

### Instances

Lists containers and VMs across every project by default, or one project
when `P` scopes down - minus the local stack's, when there's a services
panel holding those. Columns mirror `incus list`'s, with health beside
status. Type, addresses and snapshot count only appear once
`RefreshInstanceDetails` has fetched full details in the background.

Rows sort by name, with stopped instances last (`sortInstances`), and the
cursor follows the selected item across a re-sort rather than holding its
index — otherwise stopping an instance moves it down the list and hands the
selection to whatever took its place.

Which columns show, and in what order, is user-configurable via
`gui.instanceColumns`. `service`, `health` and `image` read incus-compose
config keys, so all three are blank for anything created another way.

The `image` column is that compose reference alone, having no room for
more; `Instance.Image`, which the Info tab shows, falls back to the
`image.description` the image left behind ("Alpine 3.21 arm64
(20260825_13:00)") and then to the short `volatile.base_image` fingerprint.
`presentation.GetInstanceDisplayStrings` looks up
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

- **Info** — what the instance is (name, status, type, project, image,
  health where it has one, architecture, dates, addresses, snapshot count),
  then a **Stats** section: CPU/memory/process count/disk/network from
  `InstanceFull.State`. Re-rendered every second (no extra API calls: the
  background poll already keeps that current). CPU is cumulative usage
  time, not a percentage — the API reports total nanoseconds since start,
  not a rate, and `incus info` shows the same.
- **Logs** — polls `Instance.TailConsoleLog()` and re-renders the
  accumulated buffer. See "Logs" below for why a raw snapshot doesn't work.
- **Config** — YAML dump of `api.InstanceFull`, nothing above it: identity
  is the Info tab's.
- **Env** — `environment.*` entries from `ExpandedConfig` (expanded, so
  profile-inherited variables show up too).
- **Top** — process list, polled every two seconds. Incus's API reports a
  process *count* and nothing more, so this execs `ps` inside the instance
  over the client's websocket exec (no terminal needed, unlike the `E`
  shell-out). Images without a `ps` and VMs without the guest agent get the
  daemon's error instead of a list; the flag set that works is cached per
  instance, since busybox and util-linux `ps` disagree on all of them.

### Snapshots

Follows whichever list you're in: the instances panel's `OnSelect` hands
over its instance, the services panel's the service's own — none while it
has replicas, no one of them being the service's snapshots — and the view
title carries that instance's name since the rows alone don't say whose
they are. Each panel hands the instance over rather than `refreshSnapshots`
reading the focused view, because that read takes `ViewStackMutex`, which
`switchFocus` holds while it runs an `OnSelect`: reading it there deadlocks
the app. Listing uses
`GetInstanceSnapshots` rather than the `InstanceFull.Snapshots` the
background poll already holds, because create and delete have to show up
immediately. Snapshot names come back from the API prefixed with the
instance (`alpine/snap0`); every other call wants the bare name, which
`snapshotName` strips. `n` works from the instances panel as well as
this one - it acts on the selected instance either way - and moves to the
new snapshot once it exists, so taking one from the instances panel shows
you the result. It opens a two-view popup: the editable
confirmation view as a name field, and a `snapshotOptions` view parked under
it, with `tab` moving focus between them. Its own view rather than the menu,
so the menu panel's keybindings don't fight the navigation - which means
teaching `newLineFocused`, `renderPanelOptions` and `resizeCurrentPopupPanel`
about it, the last so it keeps its position under the prompt instead of
being centred.

The options are fields rather than a list of actions: a row shows a value
that `← →` cycle in place, and enter means create wherever the focus is.
Modelling them as actions put a cursor on a checkbox, which reads as though
the row were a thing to run. The selected row is the view's cursor line, so
gocui's own highlight marks it while the options have focus; its value is
also bracketed, which is what identifies the field `← →` would change when
focus is in the name field and nothing is highlighted. Hints live in the borders the way
lazygit does it - `Subtitle` on the top, `Footer` on the bottom, the latter
needing gocui's `ShowListFooter` and skipped entirely on a view with no
lines, which is why the empty name field carries only a subtitle. Stateful is a toggle there
rather than another choice beside the expiries: the two are independent, and
a flat list of both reads as though picking an expiry rules out stateful.
Toggling reopens the menu, there being no widget with selection state -
gocui has views and keybindings, and the menu itself is lazydocker's
`SideListPanel[*types.MenuItem]`. Restore is an instance update carrying `Restore:
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
  Linux location. Which remote that is comes from the CLI config's
  default-remote, which `cliconfig` itself overrides with `INCUS_REMOTE`.
  `--remote` is that variable: `main` sets it before anything loads, since
  the client is only half of it — `incus console`, `incus exec` and the
  incus-compose verbs are subprocesses reading the environment on their
  own, and `NewCmd` hands them `os.Environ()`. See
  [docs/Remotes.md](docs/Remotes.md).
- **Connection loss**: a daemon going away mid-session is routine, so
  nothing treats it as fatal. `NoteError` classifies every error -
  `IsConnectionError` takes a `*url.Error` to mean the request never
  arrived, anything else to mean the daemon answered - and `IsConnected` is
  that verdict. The 2s instance poll drives `gui.syncConnection`, which
  raises a modal on the way down and closes it on the way back up; there's
  nothing to reconnect, since the client dials per request. `esc` dismisses
  the modal for the rest of the outage; the footer's `●`/`✗` stands either
  way. Failing to connect at startup has no client to carry on with, so
  `NewIncusCommand` returns a `ConnectError` and `App.KnownError` prints it
  rather than a stack trace.
- **Connection timeouts**: `capDialTimeout` caps both of the transport's
  dialers at 5s (a unix socket gets `DialContext`, a TLS remote
  `DialTLSContext`), the OS otherwise taking a minute or more on a host
  that's gone. The startup connect needs its own bound
  (`connectDefaultRemote`, 10s): cliconfig calls `GetServer()` before
  handing back a client, so there's no transport of ours to cap yet.
- **Errors from the main loop**: gocui ends it on any error out of a
  keybinding or an `Update` closure, which is no way to end a session, so
  `Run` sets gocui's `ErrorHandler` to `gui.handleError` - error panel for
  most things, silence for a connection error the modal already covers,
  `nil` returned either way so the loop carries on. Quitting is unaffected;
  gocui excludes `ErrQuit` before consulting it.
- **Projects**: Incus scopes instances (and networks, volumes, profiles) per
  project, and a client is scoped to one at a time. `UseProject` swaps
  `IncusCommand.client` under `clientMutex`, hence the `Client()` accessor
  rather than a field, and `GetInstances` reassigns `Instance.Client` on
  every refresh. `P` (`handleSwitchProject` in `pkg/gui/projects.go`) scopes
  to a single project.
- **All projects**: the default, and "all projects" in that menu returns to
  it. Lists every project at once through the `*AllProjects` endpoints. Each item then carries its own
  project-scoped client (`clientFor`), so actions go to the project the item
  came from - including `RefreshInstanceDetails`, which asks each instance's
  client rather than the command's, and the `incus` CLI shell-outs, which
  pass `--project`. Identity includes the project everywhere items are
  matched across refreshes: two projects can hold an instance, image or
  volume of the same name. The project column is per-panel and driven by
  `State.SpansProjects`, recomputed each refresh: a server with one project
  shouldn't carry a column repeating it on every row.
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
