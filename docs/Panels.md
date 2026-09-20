# Panels

What each side panel lists, the columns it shows, its main-panel tabs and
what its keys do. The rule they all derive from — `sidePanelDefs()`, and
what adding a panel takes — is in [CLAUDE.md](../CLAUDE.md#panels);
the keys as a user meets them are in [README.md](../README.md#usage).

## Services

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

`--project-directory` reaches this the way `--remote` reaches the daemon:
`main` sets `INCUS_COMPOSE_PROJECT_DIRECTORY`, and incus-compose resolves
it. It's validated there, since a bad path comes back as no compose project
at all and would read as the panel simply not appearing.

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

A service with more than one instance lists its replicas under it, one row
each: the panel's item is a `commands.ServiceRow`, either a service or one
of its replicas, and `ServiceRows` lays them out. `SideListPanel` is
generic over the row type, so this is a row type rather than a tree — the
list stays flat. `ServiceRow.SelectedInstance` is the one answer to "which
instance does this row mean": the replica, or a lone service's only
instance, and none from a service's own row, which means all of them. That
last case is what every tab, the snapshots panel and `withServiceInstance`
branch on. `ServiceRow.Instances` is the same split for the tabs that show an
instance each. Rows are rebuilt on every refresh, so the panel's `SameItem`
is what keeps the cursor on the row it was on; without it the selection
holds an index, which by then belongs to a different replica.

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

Keys act on the selected row's service, passing its name as the `SERVICE`
argument every incus-compose verb takes; which key runs which verb, and
which of them a replica's row takes for itself, is README's
[Compose stacks](../README.md#compose-stacks) section, alongside the rest of
what a user sees. `composeRun` is all of them, and refreshes the
instances and services panels once the subprocess returns rather than
waiting for the poll. `s`, `d` and `f` confirm, `S`/`r`/`u`/`p`/`b`/`g`
don't — same rule as the instances panel. `C` is the one key that doesn't
narrow: it runs the same verbs with the `SERVICE` argument left off, and
takes no selection, since a stack whose services have never been deployed
is brought up from there. `U` can fail on a non-local daemon for reasons
that are incus-compose's, not ours — see
[BACKLOG.md](../BACKLOG.md#caveats).

`onServiceRow` is what splits a key between the two, a replica's row taking
the instances panel's own action where one exists. Those actions are the
instances panel's, split into handler and action for this, and they refresh
both panels (`refreshInstancesAndServices`), a compose instance having a row
in each.

The keybinding menu varies with the row, which it can because it rebuilds
its bindings each time it opens. `composeRowDescription` names the other
verb where the two scopes aren't the same one, and `serviceScopedDescription`
appends "service" to the verbs only a service has — "bring up" alone, on a
replica's row, reads as bringing that replica up, which isn't a thing. The
order varies too: `servicesKeybindings` hands this panel's keys to
`orderByKey`, leading with the compose verbs on a service's own row and with
the instances panel's keys, in its order, on a replica's — the same keys
doing the same thing should be found in the same place. A key the order
doesn't name is listed last rather than dropped. The panels don't exist at
the first call, which is startup binding the keys rather than anyone reading
them.

The per-instance keys (`m`, `n`, `E`, `y`) reach the row's instance through
`withServiceInstance`, which acts directly on the one
`SelectedInstance` names and otherwise asks which — the reason
`handleSnapshotCreate` and `handleInstanceCopyIPv4` were split into handler
and action. The menu is left for a service's own row, that row meaning all
of its replicas.

Columns work the way the instances panel's do: `gui.serviceColumns` over
`serviceColumnRenderers` in `pkg/gui/presentation/services.go`, taking the
instance column names rendered from the service's instances rolled up, plus
`replicas`. That one is blank unless the service has a different number of
instances from what the compose file declared —
`presentation.ServiceReplicas` is the rule, and the Info tab's line calls it
too. It counts what exists
rather than what's running: the status column says what state the instances
are in and a replica's row says which is in which, so the figure before the
slash is the number of rows underneath, and "3/4" is one replica missing
rather than one stopped. A replica's row renders the same configured
columns through `instanceColumnRenderers` instead (`replicaCell`), indented
under the service and blank where only a service has the column, so the two
kinds of row share one table. Which columns appear at all is the service
half's to decide, replica rows following it — otherwise the rows would
disagree on how many cells they have. `gui.instanceStatusStyle` covers the
status column either way; `serviceStatusStyles` supplies the glyphs for
`partial` and `none`, which no instance state has. There's no project
column: every row shares the one project, so `servicesPanelTitle` puts it
in the title instead.

Main panel tabs:

- **Info** — what the compose file declares for the service, then the Info
  tab of each instance the row stands for, `instanceInfoStr` and all, which
  is why this renders on a ticker. Each instance is ruled off by
  `instanceHeading` and drops its own Name line, the heading having said it.
  The heading numbers replicas — "Replica 2 of 4 · web-2", the blocks being
  the same labels over and over — and numbers them over the service's
  instances rather than the ones on screen, so a replica's row still reads
  "2 of 4" while showing only itself. A service with one instance has no
  count worth printing ("Instance · redis-1"), and one carrying the
  service's own name has nothing left to say ("Instance"). It's ruled
  either way: the rule is where the compose file's half ends and the
  daemon's begins, which a service with no replicas has too.
  `instanceInfoStr` takes the identity lines to leave out, the service and
  the heading having just said them: project, image and name always, plus
  health for a lone instance. The compose fields are parsed once at startup by
  `parseComposeConfig` and stored rendered, only display wanting them; the
  Healthcheck line is the project's, from `State.ComposeProject`
  (`GetComposeProject` fetches it during `refreshServices`, so rendering
  makes no API call). The image is `ComposeService.ResolvedImage`, a
  running instance's reference before the compose file's, whose own value
  may carry no registry host. Each refresh builds new `ComposeService`
  values, so the instance count is part of `GetItemContextCacheKey` —
  otherwise the ticker goes on rendering the service object the tab opened
  with. A replica's row adds its own status to that key, the way the
  instances panel does, so a restart re-reads the log.
- **Logs** — delegates to the instance logs renderer for whichever instance
  the row names; merging several replicas' drain-on-read buffers into one
  ordered stream is a different problem, so a service's own row with
  replicas under it points at `C`'s `logs --follow` instead.
- **Env** and **Top** — the instance's own, through `serviceInstanceTab`:
  the row's replica answers for it, as does a lone service's only instance,
  and a service's own row with replicas under it says so instead — the same
  split the per-instance keys make through `withServiceInstance`.
- **Config** — both halves: a Compose section, the service's slice of
  `incus-compose config --format json` handed to `yaml.JSONToYAML`
  untouched (decoding through `map[string]any` first would turn every count
  into a float64 and render `replicas: 2` as `2.0`), then one
  `instanceConfigStr` dump per instance the row stands for, under the Info
  tab's own `instanceHeading`.

The credits tab and aggregate-logs tab from lazydocker's Project panel
aren't ported — see [BACKLOG.md](../BACKLOG.md#3-project-panel).

## Instances

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

Keybindings live in [README.md](../README.md#usage) — the canonical source,
keep that table current rather than duplicating it here. Two behaviors it
doesn't convey: `s`/`d` confirm before acting, and `p` toggles between
`freeze` and `unfreeze` depending on current status.

Main panel tabs, roughly what `incus info <name>` prints in one shot, split
up:

- **Info** — what the instance is (name, status, type, project, image,
  health where it has one, architecture, dates, addresses, snapshot count),
  then the counters from `InstanceFull.State`: CPU, memory, process count,
  disk, network. A gap sets those off rather than a heading — CPU and
  memory say what they are, and a ruled heading would weigh the same as the
  replica headings stacking whole instances, a level above it. Both halves
  pad their labels to `identityPadding`, the tab being one run of them.
  Re-rendered every second (no extra API calls: the background poll already
  keeps that current). CPU is cumulative usage
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

## Snapshots

Follows whichever list you're in: the instances panel's `OnSelect` hands
over its instance, the services panel's whatever its row stands for — a
replica's own, and none at all from a service's row with replicas under it,
no one of them being the service's snapshots — and the view title carries
that instance's name since the rows alone don't say whose they are. Each panel hands the instance over rather than `refreshSnapshots`
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

## Images

Local images (`GetImages`), identified by `Image.Label()`: alias, else the
description an unaliased cached image carries (what `incus image list` shows
in its DESCRIPTION column), else the short fingerprint. Truncated to keep
the columns after it on screen. `d` deletes after a confirmation. Polled every 10s rather than the instance list's 2s: images
only change when someone pulls or deletes one.

## Volumes

Every storage pool's volumes in one list (`GetVolumes` walks
`GetStoragePoolNames` then `GetStoragePoolVolumes` per pool; a pool that
errors is skipped rather than emptying the panel). Identity is
pool+type+name, since an instance and a custom volume can share a name.
Only `custom` volumes can be deleted — the rest go away with the instance or
image they belong to.

## Networks

`GetNetworks`, managed and unmanaged alike. Only managed ones can be
deleted; the unmanaged entries are host interfaces Incus merely reports.
