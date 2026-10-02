# Panels

What each side panel lists, the columns it shows, its main-panel tabs and
what its keys do. The rule they all derive from — `sidePanelDefs()`, and
what adding a panel takes — is in [CLAUDE.md](../CLAUDE.md#panels);
the keys as a user meets them are in [README.md](../README.md#usage).

## Confirmations

A prompt that stops, deletes or restores something names the item's
project whenever its list holds more than one project's (`qualified`):
the same image alias, profile name or instance name can exist in several,
and "delete image nginx:alpine?" doesn't say which is about to go. With
one project the name stands alone. One for a stack on another remote than
the session's names that remote too - see
[Stacks on other remotes](#stacks-on-other-remotes). A delete the daemon would refuse (see
[docs/Incus.md](Incus.md)) is said instead of asked; in-use goes by the
listing's `used_by`, which events and the polls keep current, and the
daemon has the last word either way.

## Where an item lives

Every Info tab, and the header of every Config tab that has one, starts
with where the item lives, broadest first (`locationStr`): its remote -
always, the footer's being the session's and not necessarily the
item's - then its project, and for an instance incus-compose made, its
stack and service. incus-compose names the project for the stack, so the
Stack line doesn't repeat the name: it's the directory the stack is
listed from (`stackDirs`, which each Stacks refresh publishes for the
tabs, rendering off the main loop as they do), or "not listed". Which
replica an instance is stays its name's to say, and the replica headings'
on Service Info. A resource is the session's remote, the only one those
panels list. Lines the tab's heading has already said are left out: a
service's instance blocks repeat none of it.

## Copying

`y` on any list opens a menu of what the item has to copy, built by
`copyMenu` from label/value pairs, each value shown beside its label so
the menu says what lands on the clipboard; a value the item lacks - a
stopped instance's address - is left out rather than offered empty. A
network's addresses are Incus's `ipv4.address`/`ipv6.address`, the
gateway with its prefix, and labelled as such rather than as the subnet.

## What keeps them current

Two things keep the screen current: the daemon's event stream, which
refreshes a list the moment something changes, and polls, for what no
event reports. Why each event counts, and the daemon's quirks, are in
[docs/Incus.md](Incus.md), "Events"; `eventRefreshes` in
`pkg/gui/events.go` is the full list.

The lists. A burst of events is gathered over 200ms into one refresh, and
an `instance-updated` refreshes at most every 10s.

| List | Refreshed by | Polled |
|---|---|---|
| Instances | `instance-created`, `-deleted`, `-renamed`, `-started`, `-stopped`, `-shutdown`, `-restarted`, `-paused`, `-resumed`, `-restored`, `-migrated`, `instance-agent-started`/`-stopped`, `instance-snapshot-created`/`-deleted`/`-renamed`; `instance-updated`; an operation ending | every 2s, stream or not |
| Stacks | whatever refreshes the instances; a compose verb; another remote connecting, or answering differently | every 10s; once a minute while the stream is open, unless a stack is on another remote |
| Services | whatever refreshes the instances | with the instances, every 2s |
| Snapshots | an instance's come with the instances listing, a volume's with the volumes | with those lists |
| Images | `image-created`/`-deleted`/`-updated`/`-refreshed`, `image-alias-*`; `instance-created`/`-deleted`/`-renamed`, for the used-by count | every 10s; once a minute while the stream is open |
| Volumes | `storage-volume-created`/`-deleted`/`-renamed`/`-updated`/`-restored`, `storage-volume-snapshot-created`/`-deleted`/`-renamed`/`-updated`, `storage-pool-created`/`-deleted`/`-updated`; `instance-created`/`-deleted`/`-renamed`, `instance-updated` | as Images |
| Networks | `network-created`/`-deleted`/`-renamed`/`-updated`, `network-forward-*`, `network-acl-*`; `instance-created`/`-deleted`/`-renamed`, `instance-updated` | as Images |
| Profiles | `profile-*`; `instance-created`/`-deleted`/`-renamed`, `instance-updated` | as Images |

An operation event also marks a row the moment the daemon takes the
action, whoever asked: `starting`, `stopping`, `restarting`, `restoring`,
`freezing` or `unfreezing`, until a listing taken after it ends lands.
When the stream opens, every list is fetched again for what changed while
none was; when it drops, the marks come off, the instances are listed at
once to see whether the daemon has gone, and the polls return to 10s.

Only a poll catches:

- an instance's CPU, memory, processes and addresses - the 2s poll;
- an instance snapshot edited, its expiry say, which sends no event -
  the 2s poll;
- a volume's usage and an image's last use - once a minute while the
  stream is open;
- a network's leases and state - their own tabs' tickers.

The main-panel tabs. A tab that ticks reads again on its own; one drawn
once is drawn again when a refresh changes the selected item's cache key,
which carries a fingerprint of what the tab shows - whether or not the
list has focus, so a tab being read follows too.

| Tab | Drawn | Follows a change through |
|---|---|---|
| Instance Info | every 1s | the newest instances listing |
| Instance, service, stack Logs | every 1s | its own console-log read |
| Instance Config, Env | once | the instance's config, less ic-healthd's `user.healthcheck.*` verdicts, which change every few seconds |
| Instance, service Top | every 2s | its own `ps` |
| Stack Info | every 1s | the newest stacks listing, and all but its first lines the newest services listing |
| Stack Config | once | the stack's directory and project |
| Service Info | every 1s | the newest services listing, and each instance's block the newest instances listing |
| Service Config | once | every one of its instances' config |
| Snapshot Config | once | the snapshot |
| Image Config | once | the image and what uses it |
| Volume Config | once | the volume, less its usage, which changes with every write |
| Network Leases | every 5s | its own read |
| Network State | every 2s | its own read |
| Network ACLs, Forwards | once | a `network-acl-*` or `network-forward-*` event, neither being part of the network |
| Network Config | once | the network |
| Profile Devices, Config | once | the profile |

## Stacks

One row per compose stack, a stack being a directory with a compose file
in it: the local one, and every one saved with `a`. Stacks and Services are
there whenever `incus-compose` is on `PATH` — `Run` asks `exec.LookPath`
once, ahead of `createAllViews` — and then Stacks is `[1]`, with the focus
when a stack is on the session's remote, and Services `[2]`. Without it
neither panel exists and the layout is what it always was.
`SideListPanel.Hide` is what removes them, and the layout already copes:
`setViewFromDimensions` marks a view with no box invisible.

That gate means panel numbering can't index `sidePanelDefs()` — a hidden
first panel would leave a hole at `[1]`. `visibleSidePanelDefs` is what the
number keys, the title prefixes, `sideWindowNames` and the startup focus all
run over instead. Hidden-ness is a `hidden` func on the def rather than the
panel's own `Hide`, because views are styled and keys bound before
`setPanels` has built any panel to ask; and it's fixed for the session,
the number keys being bound once.

A remote with no stack listed on it - every stack pinned elsewhere, or none
at all - keeps both panels but collapses them to their titles
(`State.StacksHere`, set by each Stacks refresh), Stacks counting what it
lists and Services leaving out the stack (`titleStacks`). Focusing either
expands both and collapses Instances instead (`stacksCollapsed`), and
focus crossing that swap lands on the panel's first main-panel tab
(`stacksSwap`). Their numbers don't move: a hidden panel would renumber
everything by remote. The Stacks refresh after startup or a remote switch
(`State.Landing`) also moves the focus from either one to Instances;
adding or removing a stack changes only the layout.

The local stack is the working directory's, or the one `--project-directory`
names. The flag reaches this the way `--remote` reaches the daemon: `main`
sets `INCUS_COMPOSE_PROJECT_DIRECTORY`, validated there, and
`localStackDir` reads the variable back. A working directory with no
compose file is no stack at all rather than a row saying so — most people
start lazyincus from somewhere that isn't one — but a directory named with
`-P` that has none is a row with the error. The local stack isn't saved, so
`D` and `e` refuse it. The saved ones are `state.yml`'s
([docs/Config.md](Config.md#state)); `a` adds to it, `e` replaces an entry
in place (`ReplaceStack`), through `a`'s prompt pre-filled and its
checks, and `D` removes from it,
re-reading the file before each write so two sessions don't undo each
other, and replacing it by rename. `a` takes a path through
`openTextPrompt`, a one-line prompt in the confirmation view
(`pkg/gui/prompt_panel.go`), and keeps it only once `incus-compose config`
has read a project out of it: `~` and a relative path are resolved, and a
path that's missing, not a directory, holds no compose project or is
already listed is refused.

A stack's config is one `incus-compose config --format json` in its
directory (`LoadComposeStack`), read in `fetchStacks` rather than before
the views exist, and cached by directory (`stackCache`): a subprocess per
stack on every refresh would be most of the cost of one. The cache forgets
a stack whenever a compose verb runs on it, so a compose file edited and
then brought up shows its new services - and after `c`, which opens the
file compose-go would pick (`ComposeFile`): the first of `compose.yaml`,
`compose.yml`, `docker-compose.yml`, `docker-compose.yaml`, in the
directory or the nearest one above with one. An override file or `-f` in
the environment isn't followed. A read that failed is tried again
on each refresh — the directory may come back — except the working
directory's. `ComposeCmd` is how every incus-compose subprocess is made: in
the stack's directory, with `INCUS_COMPOSE_PROJECT_DIRECTORY` set to it as
well, since `-P` sets that for the whole process and would otherwise point
every stack at the one it names.

The status column rolls up every compose-labelled instance in the stack's
project the way a service rolls up its replicas (`RollUpStatus`): theirs
when they agree, `partial` when they don't, `none` with nothing there,
`error` for a stack whose config couldn't be read, and - for a stack on
another remote, below - `connecting` or `unreachable`. The path column
writes home as `~`.

`followStack` is how the services panel follows the selection, the way
Snapshots follows the instances panel's. Stacks' `OnSelect` calls it, and
so does every Stacks refresh, the selection being the first row until
someone moves it. It keeps the stack in `gui.selectedStack`, an atomic
pointer, since `fetchServices` reads it off the main loop. A different
stack — a different directory or remote, or a project the compose file
now names differently — invalidates the services `refreshSeq`, empties the panel,
retitles it and fetches for the new one. The stack is swapped before the
invalidation, and `fetchServices` takes its ticket before reading the
stack, so a fetch that isn't turned away has the new one.

Stacks refresh on the 10s cadence of the other lists that events keep
current (`pollUnlessWatched`), on any event that refreshes the instances,
and after a compose verb. While a stack pinned to another remote is listed
they stay at 10s with the stream open: that remote's events never reach
this session. The keys are the Services panel's compose verbs
with the `SERVICE` argument left off, through the same code,
parameterised by `composeTarget`. `p` pauses unless every service with
anything running is frozen. Which key runs which verb is README's
[Compose stacks](../README.md#compose-stacks) section.

Main panel tabs:

- **Info** — laid out like an instance's: the remote, project and
  directory, where the stack is listed from (at startup, saved, or both),
  and its status - or, while its remote is connecting or away, why - then
  the declared services counted by status and what's rolled up over its
  instances: health, dates, snapshots. Under headings of their own:
  **Endpoints**, each instance's address and the ports its proxy devices
  publish (incus-compose's `ports:`, listening on the daemon's host, so a
  wildcard reads as the stack's remote's host - `publishHostFor` - or `*`
  where its URL doesn't give one: a unix socket, or loopback, which an SSH
  tunnel to the API is); **Usage**, the instance counters summed, a custom
  volume shared by replicas counted once; and **Drift**, compose instances
  whose service the file no longer declares and declared services with
  none. Everything
  past the services count comes from `gui.composeInstances` and
  `gui.composeProject`, which `fetchServices` fills for the selected
  stack, so the tab ticks until they arrive.
- **Logs** — every instance's console log, services in name order, each
  under a heading naming its replica, or its service when it has only the
  one instance. `m` jumps here, as it does on the other panels.
- **Config** — the whole of `incus-compose config`, through JSON to YAML
  for the reason the service's Config tab gives.

### Stacks on other remotes

A `remote:` ahead of the path pins a stack to that remote
(`SplitStackInput`), but only when the prefix is one of the CLI's instance
remotes, so a path with a colon in it still reads as a path. With none,
it's pinned to the session's, so a session on another remote later can't
run its verbs on the wrong daemon. It's saved as `remote:/dir`
(`ComposeStack.Ref`), its identity everywhere a directory alone was: the
same directory can be listed once per remote. `Run` pins an entry saved
before stacks had remotes to the remote it starts on (`PinStacks`), the one
each was shown against until then, which leaves the local stack, never
saved, the only one that follows the session. `listStacks` keys every
stack by the remote it's on now, so the local stack and the same directory
saved for the session's remote are one row, named by the saved entry so
`D` and `e` act on it - `D` forgetting the entry, the row staying as the
local stack, which its confirmation says. Whether a remote is the session's is asked only
where it matters (`onSessionRemote`), so starting on another `--remote`
never changes what's saved, and a remote that's known but doesn't answer
is still added: it's the remote's to fix.

`gui.commandFor` gives a stack on another remote an `IncusCommand` of its
own (`pkg/gui/remotes.go`), through which its statuses, its services and
the Info tab's `PublishHost` go. Nothing a poll runs waits on another
server: the first ask starts connecting in the background and answers
`errConnecting`, the row reading `connecting`; a failed connect isn't
tried again for 30s; and that remote's statuses are the last ones read,
read again off to one side (`cachedStatuses`), the Stacks and Services
refreshing when a connection lands or a read changes something. The
session's own are read in the refresh, as every other list is. Services
skip a remote whose statuses last failed, the list saying so in place of
rows (`EmptyNote`), and a remote gone from the CLI's
config - renamed since the stack was saved - isn't tried at all
(`unknownRemote`), its message pointing at `e`. The compose verbs and the
instances' `incus console`/`exec`/`config edit` need no connection, only
`INCUS_REMOTE` set to the stack's remote (`commands.WithRemote`).

Everything else - Standalone Instances, Snapshots, Resources, the event
stream, the connection-lost modal - is the session's remote. So a stack
pinned elsewhere has instances that were never in Standalone Instances to
be filtered out, and wherever the rest of the app would vouch for the
wrong daemon - the Services and Snapshots titles, the new-snapshot prompt,
the compose and instance confirmations - `onRemote` adds "on pve01".
`space` (`stackSwitchRemote`) is `R`'s switch to the stack's remote, on a
connection its status has usually opened already: a key rather than the
selection, since a switch reloads every panel.

A remote column leads the Stacks rows whenever one is on a remote other
than the session's (`State.StacksElsewhere`), the session's marked `*` in
green as the `R` menu marks it - the project columns' rule, a column only
where the rows would otherwise read alike. The local stack sorts first
when it's saved nowhere, then each saved remote's stacks together, each by
name: by what's saved alone, the local stack that is also a saved entry
included, since which entry that is depends on the session's remote and
switching would reshuffle the list under the cursor.

## Services

The services of the stack the Stacks panel has selected — see
[Stacks](#stacks) for when the panel is there and how it follows the
selection. With no stack, it says how to add one.

Rows come from the stack's config, not from the daemon: `parseComposeConfig`
reads `.name` and `.services`, so a service the compose file declares but
nothing is running still gets a row, in state `none`.
`GetComposeServices` then pairs each declared service with the project's
instances, matching on `user.label.incus-compose.service` via
`Instance.ComposeService()`, and records the stack's directory and remote
on each service for its Config tab and its verbs. It fetches with
`GetProjectInstances`, through the stack's remote's command
(`commandFor`), rather than reading the instances panel's list, since the
panels can be scoped anywhere, and on another remote altogether. An
instance whose label names no declared service — a one-off from
`incus-compose run` — matches no row and stays in the instances panel.

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
instances. `Health` rolls up ic-healthd's verdict on the running replicas, worst-first.

The instances panel is the other half of the split: its filter
(`isStackInstance`) drops the instances of every service a listed stack on
the session's remote declares, whichever stack is selected, and its title
becomes "Standalone Instances" while any such stack is listed. A stack on
another remote has nothing there to drop. A compose instance no Services row
claims stays there, having no panel of its own: one of a project no stack
lists, or of a service its compose file no longer declares, which the
stack's Drift names. `C` sets `ShowStackInstances`, which turns the filter
off and the title back to "Instances" until it's pressed again; it's
bound only when incus-compose is there to have stacks.
`SpansProjects.Instances` is computed over what's left after that filter,
not over everything the daemon returned, and again whenever the stacks
change which services they declare, or `C` is pressed. The stacks are fetched first at startup,
so the instances panel doesn't show a stack's rows and then take them away.

Because the stacks have panels of their own, startup doesn't scope the
client to one — the instances panel spans projects like every other panel,
and `P` still narrows.

Keys act on the selected row's service, passing its name as the `SERVICE`
argument every incus-compose verb takes; which key runs which verb, and
which of them a replica's row takes for itself, is README's
[Compose stacks](../README.md#compose-stacks) section, alongside the rest of
what a user sees. `composeRun` is all of them, run in the service's stack
directory (`ComposeCmd`) with `INCUS_REMOTE` naming the stack's remote,
and refreshes the instances, stacks and services
panels once the subprocess returns rather than waiting for the poll.
`s`, `d` and `f` confirm, `S`/`r`/`u`/`p`/`b`/`g`
don't — same rule as the instances panel. The same verbs over the whole
stack are the Stacks panel's.
`U` can fail on a non-local daemon for reasons
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

The per-instance keys (`n`, `c`, `E`, `y`) reach the row's instance through
`withServiceInstance`, which acts directly on the one
`SelectedInstance` names and otherwise asks which — the reason the actions
behind them (`snapshotCreatePrompt`, `instanceCopy`) take the instance
rather than reading the selection. The menu is left for a service's own row, that row meaning all
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
in the title instead, followed by the stack's remote when that isn't the
session's.

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
  the heading having just said them: where it lives, image and name
  always, plus health for a lone instance. The compose fields are parsed with the stack's
  config by `parseComposeConfig` and stored rendered, only display wanting
  them; the Healthcheck line is the project's, from `gui.composeProject`
  (`GetComposeProject` fetches it in `fetchServices`, so rendering makes no
  API call; the tab renders off the main loop, so it's an atomic pointer). The image is `ComposeService.ResolvedImage`, a
  running instance's reference before the compose file's, whose own value
  may carry no registry host. Each refresh builds new `ComposeService`
  values, so the instance count is part of `GetItemContextCacheKey` —
  otherwise the ticker goes on rendering the service object the tab opened
  with. A replica's row adds its own status to that key, the way the
  instances panel does, so a restart re-reads the log.
- **Logs** — delegates to the instance logs renderer for whichever instance
  the row names; a service's own row with replicas under it stacks all of
  their logs, each under the Info tab's `instanceHeading`. They stay
  separate streams — the buffers carry nothing to interleave them on — so
  `M`, `logs --follow`, is the merged view.
- **Env** and **Top** — the instance's own, through `serviceInstanceTab`:
  the row's replica answers for it, as does a lone service's only instance,
  and a service's own row with replicas under it says so instead — the same
  split the per-instance keys make through `withServiceInstance`.
- **Config** — both halves: a Compose section, the service's slice of
  `incus-compose config --format json`, read afresh in its stack's
  directory, handed to `yaml.JSONToYAML`
  untouched (decoding through `map[string]any` first would turn every count
  into a float64 and render `replicas: 2` as `2.0`), then one
  `instanceConfigStr` dump per instance the row stands for, under the Info
  tab's own `instanceHeading`.

The credits tab and aggregate-logs tab from lazydocker's Project panel
aren't ported — see [BACKLOG.md](../BACKLOG.md#3-project-panel).

## Instances

Lists containers and VMs across every project of the session's remote by
default, or one project when `P` scopes down - minus the instances of the
stacks listed for that remote, which the services panel holds (see
[Services](#services)). Columns mirror `incus list`'s, with health beside
status.

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
`incus pause` and `incus resume` depending on current status.

Main panel tabs, roughly what `incus info <name>` prints in one shot, split
up:

- **Info** — where the instance lives ([above](#where-an-item-lives)),
  then what it is (name, status, type, image, health where it has one,
  architecture, dates, addresses, snapshot count),
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
  is the Info tab's. Drawn once, and again when the config changes (see
  [What keeps them current](#what-keeps-them-current)).
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
replica's own, and every replica's from a service's row above them, the
service's snapshots being all of theirs. Stacks hands over nothing, which
empties it. The view title names what the rows
belong to, the instance or the service - and its remote, when that isn't
the session's - since the rows alone don't say; a
panel holding more than one instance's snapshots grows a column naming the
replica each came from, and groups the list by instance before ordering it
newest-first, replicas being snapshotted alike. Each panel hands its
selection over rather than `renderSnapshots`
reading the focused view, because that read takes `ViewStackMutex`, which
`switchFocus` holds while it runs an `OnSelect`: reading it there deadlocks
the app. The rows are the `InstanceFull.Snapshots` the instance listing
already carries, so moving through a list asks the daemon for nothing;
create, delete and restore re-run that listing, which is what shows their
result at once. Snapshot names can come back prefixed with the instance
(`alpine/snap0`); every other call wants the bare name, which
`snapshotName` strips.

`e` swaps the selection for every instance the instances panel holds, the
stacks' replicas included (`gui.showAllSnapshots` picks which way a
session starts). The panels go on handing their selection over while it's
on, so turning it off lands on whatever is selected by then; they just
don't rerender a list that wouldn't change. Rows group by project before
instance, and name the project with the instance once the list spans
projects, instance names being unique only within one. `n` from this panel
then snapshots the selected row's instance, not the instances panel's, and
the new snapshot is found by project, instance and name, since every
replica of a service can carry one of the same name.

`n` works from the instances panel as well as
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
lines, which is why the empty name field carries only a subtitle. A
popup with a subtitle widens, still centred, to fit it beside the title
(`popupFrameWidth`): gocui right-aligns the subtitle on the border the
title starts, and the middle half of a narrow screen ran the two together.
Stateful is a field of its own, flipped by `← →` like the rest, rather
than another choice beside the expiries: the two are independent, and a
flat list of both reads as though picking an expiry rules out stateful.
Restore is an instance update carrying `Restore: <name>`, not a snapshot
operation.

A custom volume selected in the volumes panel is followed the same way,
its title naming the volume, and `n` there or here snapshots it. A
volume's snapshots don't come with the volume listing the way an
instance's do, so the volumes fetch asks for each custom volume's in the
same fan-out as the sizes; moving through the list still asks for
nothing. The other volume types are left alone - their snapshots are
their instance's - and selecting one leaves the panel on what it was
showing. `commands.Snapshot` is either kind, `Volume` set on a volume's,
and create, restore and delete take the volume calls for it; restore is
likewise an update of the volume carrying `Restore`. The `n` popup drops
its stateful field for a volume, which has no runtime state. The panel's
all-instances view stays instances only.

## Resources

Images, volumes, networks and profiles are four panels sharing one window: a
`window` on their defs puts them in the same slot, and `window.go` is what
knows about it. The views are stacked at the same position and the layout
shows the window's active one, which is whichever was focused there last
(`switchFocusAux` notes it). Their titles are gocui `Tabs` - the same list
on each, each with its own `TabIndex` - so the title reads as the window's
rather than the list's. When the names don't fit the title, `fitWindowTabs`
swaps in each def's `shortTitle` (`Img - Vol - Net - Prof`) on the layout
pass, so a resize refits them: a tab cut off the end is a list nobody knows
is there. Number keys, `tab` and the side column's split
all count windows, so the three take one number and one share of the
height; they read something far less often than instances do.

`←`/`→` and `h`/`l` step list by list, lazydocker's way, so the three are
stops of their own there, where `tab` and the number keys stop at the
window; `[`/`]` stay the main panel's tabs, which Networks has three of.
The window's number key pressed again moves to its next list, and a click
on a tab's name to that one. The arrows are global bindings, and the
main panel's own - scrolling sideways - win while it has focus. It's a
focus change
like any other, so a filter on one list is dropped on moving to the next,
as it is moving between any two panels. The hidden lists keep polling,
so a switch shows current rows at once.

`u` on any of the three narrows the instances panel to what uses the item
and moves there, its title naming it; `esc` there brings the rest back and
returns to the list `u` was pressed in, cursor where it was.
While narrowed it shows every user, stopped or a stack's, the
question being what uses the thing. An image's users are its `UsedBy`.
A network's or volume's `used_by` names a profile rather than the
instances that have it, so those match on each instance's expanded
devices - a NIC's `network` or `parent`, a disk's `pool` and `source` -
as well as any instance `used_by` names outright; an entry there without
a project is default's. A network or volume in a project other than
default only counts that project's instances.

`c` hands the terminal to `incus image edit`, `incus storage volume edit`
or `incus network edit` with the item's `--project` - and on the instances
and services panels to `incus config edit`, a service's own row asking
which replica the way `E` does - the way `E` hands it
to `incus exec`: the CLI opens the YAML in `$EDITOR` and re-opens it when
it doesn't validate, which a form of our own would have to reinvent per
resource. The list re-fetches when the editor exits. A host interface has
no config to edit, so `c` there says so.

## Images

Local images (`GetImages`), identified by `Image.Label()`: alias, else the
description an unaliased cached image carries (what `incus image list` shows
in its DESCRIPTION column), else the short fingerprint. It takes whatever
width the columns after it leave, down to 28 characters and cut with an
ellipsis below that - one of the `FlexColumns` a side panel can name, which the
volumes' and snapshots' names are too, and the instances' and services'
`image` column wherever the config puts it. With more than one they give
way in the order named, each to its own floor: the snapshots panel's
instance column goes first, repeating down a list grouped by it, before
the snapshot name you act on. `d` deletes after a confirmation. Refreshed
by the image events rather than the instance list's 2s poll: images only
change when someone pulls or deletes one, and a slower poll backs the
events up (see [docs/Incus.md](Incus.md), Events).

Each image carries the instances created from it (`Image.UsedBy`), matched
on `volatile.base_image`, and the count is the column after the label - red
at 0 - since whether an image can go is what the panel is for. The images
fetch lists instances across every project for this, whatever the panels
are scoped to: a project without `features.images` uses default's images,
so an image one project lists can be another's instances' base. A client
refused the all-projects listing falls back to its own project's; one that
can't list instances at all still gets its images, their users `?` and
none of them offered to prune.
Instances are matched by fingerprint alone, so the per-project copies
incus-compose makes of an image all count the same users. Then size, the
date an instance was last created from it, and `vm` or `cached` where they
apply; the container type every other image has isn't worth a column.

`D` prunes: a menu of the unused cached images - what Incus cached on a
launch and expires by itself - or every unused one, each with its count
and size, then a confirmation naming them all. The second is the one that
matters with incus-compose, whose copies aren't cached images and are
never expired: an old tag stays until someone deletes it. Deletes run one
at a time and a failure doesn't stop the rest; what failed is listed once
they're done.

## Volumes

Every storage pool's volumes in one list (`GetVolumes` walks
`GetStoragePools`, which brings each pool's driver too, then the volumes
of each - every project's at once while the panels span them; a pool that
errors is skipped rather than emptying the panel). Identity is
pool+type+name, since an instance and a custom volume can share a name.
Only `custom` volumes can be deleted — the rest go away with the instance or
image they belong to.

After the name come the users count - red for a custom volume nothing has
attached, the other types always belonging to something - and the size,
the two a narrow panel should keep. Sizes are a request each
(`GetStoragePoolVolumeState`), eight in flight at a time, on every poll;
where the driver can't size a volume the cell is blank (see
[docs/Incus.md](Incus.md)). Each pool's space is one more request a poll,
shown on the Config tab with the driver, and so are the users by name:
`Volume.Users` turns the used_by URLs into an instance's name, or a
profile's kind and name, with the project where it isn't the volume's.

## Networks

`GetNetworks`, managed and unmanaged alike, with the unmanaged ones - host
interfaces Incus merely reports, and only managed networks can be deleted -
filtered out until `e` shows them, the way `e` shows stopped instances
and every instance's snapshots: `e` is each list's "show what's left out".
Not `a`, which Instances spends on attach, lazydocker's key for it too.
They outnumber Incus's own networks on most hosts and have nothing to do.

The tabs are Leases, State, ACLs, Forwards and Config. Leases is a host a row, IPv4 and
IPv6 side by side, the gateway first: the daemon lists an entry per
address, which would give a dual-stack instance two half-rows. Asking for
them takes a request per project using the network (see
[docs/Incus.md](Incus.md)). State is `incus network info`, with a bridge's
ports named by the instance and NIC on the other end. Both are ticker
tabs, leases every 5s and state every 2s: an instance starting takes a
lease without changing anything the list would notice, and no event says
it did.

ACLs is what filters the network's traffic: the ACLs `security.acls`
applies to the network, what becomes of traffic none of their rules match
(`security.acls.default.*.action`, `reject` when unset), the NICs on the
network carrying ACLs of their own - found the way `u` finds a network's
users, through each instance's expanded devices - and then every one of
those ACLs' rules, ingress and egress. Rendered once, not polled: ACLs
change when someone edits one, and a `network-acl-*` event says so -
forwards and ACLs being no part of the network, the event bumps
`gui.networkTabs`, which the cache key carries, and the tab is drawn again.

Forwards is `incus network forward list` a port to a row: the listen
address, protocol and port, where it goes - the target port defaulting to
the listen port, as the daemon's does - and the instance holding that
address, from the instances' own addresses. A forward's `target_address`,
which takes every port no entry names, gets a row of its own. Load
balancers aren't shown; they're OVN's alone. Redrawn on a
`network-forward-*` event, the way ACLs is.

## Profiles

Every project's profiles in the all-projects view, each project having
its own `default`, so the project column is there more often than not.
After the name, how many things use it and the names of the devices it
hands out, then the description, which is the column that gives way.
Devices is the first tab, a device a row with its settings as
`key=value`: what an instance on the profile gets.

`u`, `c` and `d` are the other resources' keys. `u` matches an instance
that names the profile in its own list or is named in the profile's
`used_by`, and - as with networks and volumes - a profile in a project
other than default only reaches that project's instances. `c` is
`incus profile edit`. `d` never asks about a project's `default`, which
the daemon won't delete by name, nor about a profile in use - see
[Confirmations](#confirmations) - and only ever takes the one project's.
