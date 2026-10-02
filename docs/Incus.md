# Talking to Incus

The non-obvious parts of talking to the daemon. Most of these were
established against a live daemon or read out of the Incus source, not
inferred.

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
  [docs/Remotes.md](Remotes.md).
- **Connection loss**: a daemon going away mid-session is routine, so
  nothing treats it as fatal. `NoteError` classifies every error -
  `IsConnectionError` takes a `*url.Error` to mean the request never
  arrived, anything else to mean the daemon answered - and `IsConnected` is
  that verdict. The 2s instance poll drives `gui.syncConnection`, which
  raises a modal on the way down and closes it on the way back up; there's
  nothing to reconnect, since the client dials per request. The event
  stream gets there sooner without judging itself - a dropped websocket
  isn't a `*url.Error` - by asking for an instance listing the moment it
  drops, and syncing after the listing its reopening brings. `esc` dismisses
  the modal for the rest of the outage; the footer's `●`/`✗` stands either
  way. Failing to connect at startup has no client to carry on with, so
  `NewIncusCommand` returns a `ConnectError` and `App.KnownError` prints it
  rather than a stack trace.
- **Connection timeouts**: `capDialTimeout` caps both of the transport's
  dialers at 5s (a unix socket gets `DialContext`, a TLS remote
  `DialTLSContext`), the OS otherwise taking a minute or more on a host
  that's gone. Connecting needs its own bound
  (`connectRemote`, 10s): cliconfig calls `GetServer()` before
  handing back a client, so there's no transport of ours to cap yet.
  The event stream's websocket dials through the same capped dialers but
  takes no context, so `ListenForEvents` doesn't wait on a dial it's been
  cancelled during - `run` waits for the event watcher, and quitting
  would otherwise sit out the 5s - and closes the stream if it opens late.
- **Errors from the main loop**: gocui ends it on any error out of a
  keybinding or an `Update` closure, which is no way to end a session, so
  `Run` sets gocui's `ErrorHandler` to `gui.handleError` - error panel for
  most things, silence for a connection error the modal already covers,
  `nil` returned either way so the loop carries on. Quitting is unaffected;
  gocui excludes `ErrQuit` before consulting it.
- **Projects**: Incus scopes instances (and networks, volumes, profiles) per
  project, and a client is scoped to one at a time. `UseProject` swaps
  `IncusCommand.client` under `clientMutex`, hence the `Client()` accessor
  rather than a field, and every refresh builds its `Instance`s with a
  client of their own. `P` (`handleSwitchProject` in `pkg/gui/projects.go`) scopes
  to a single project.
- **All projects**: the default, and "all projects" in that menu returns to
  it. Lists every project at once through the `*AllProjects` endpoints. Each item then carries its own
  project-scoped client (`clientFor`), so actions go to the project the item
  came from - the `incus` CLI shell-outs included, which pass
  `--project`. Identity includes the project everywhere items are
  matched across refreshes: two projects can hold an instance, image or
  volume of the same name. The project column is per-panel and driven by
  `State.SpansProjects`, recomputed each refresh: a server with one project
  shouldn't carry a column repeating it on every row.
- **List instances**: `GetInstancesFull` / `GetInstancesFullAllProjects`
  (recursion 2, what `incus list` itself asks for) return every instance's
  config, state and snapshots in one request, cheap enough for the
  2-second poll. The alternative, a plain list plus `GetInstanceFull` per
  instance, is one request per instance per tick.
- **Events**: `GetEventsAllProjectsByType`, or `GetEventsByType` for one
  project, is a websocket the daemon sends lifecycle events down as things
  change, whoever changed them. lazyincus keeps one open for the scope the
  panels list, reopening it after a project switch and, backing off, after
  a drop, and refreshes the lists an event touches once a 200ms burst has
  passed. Only events that change a list count, named one by one: the
  daemon also sends them for reads, an `instance-exec` for every `ps` the
  Top tab runs, so matching by prefix would have each refresh set off the
  next. `instance-updated` - a device attached, a profile added - refreshes
  the lists counting what uses a volume, network or profile, but at most
  every 10s: ic-healthd sends one per instance each time it records a
  healthcheck. `instance-agent-started` refreshes the instances: a VM's
  state comes from its agent once there is one (`renderState` in the qemu
  driver), and until then from the host side - on 7.4 a restarted VM's
  address read `eth0` within 2s, then the guest's own `enp5s0` once the
  agent was up, 7s later. An instance snapshot's edit sends no
  `instance-snapshot-updated`, whatever `api` declares: `snapshotPut`
  calls `Update(args, false)`, and that `false`, `userRequested`, is what
  the event hangs on; only the "Updating snapshot" operation says so. A
  volume snapshot's edit does send `storage-volume-snapshot-updated`. The
  polls stay, for what no event reports - CPU, memory, a DHCP address -
  and slow down while a stream is open. What each event refreshes, and
  what's left to the polls, is the table in
  [docs/Panels.md](Panels.md#what-keeps-them-current).
  `*incus.EventListener` has unexported fields, so `commands.EventListener`
  is the interface in front of it that `incustest` implements. An
  operation event arrives the moment the daemon takes an action, whoever
  asked for it, naming the instances it acts on, and again when it's done:
  a CLI restart of a VM on 7.4 read `Restarting instance` Running, then
  Success with the `instance-restarted` 1.6s later. The description is the
  operation's only name, so `operationStatuses` matches those strings; one
  it doesn't know marks nothing. Order takes care on both ends. The client
  runs each `AddHandler` call on a goroutine of its own, so a handler can
  see an operation's Running after its Success - seen on 7.4 as Running
  before Pending - and a mark nothing would end; `ListenForEvents` reads
  `AddChannel` instead, which keeps the daemon's order. And the daemon's
  `Start` (`internal/server/operations`) sends Running only after setting
  the work off, which a quick operation can finish first, so the last
  hundred operations to end are remembered and a Running for one of them
  marks nothing. When the stream drops, the marks of
  operations still under way come off, no event being left to end them.
  Under `--debug` every event the stream delivers, the ones that change
  nothing included, goes to `development.log` in the config directory,
  with a line each time a stream opens naming what it listens to - the
  place to look when a list doesn't refresh.
- **Instances are values**: each refresh builds new `*Instance`s rather
  than updating the last ones in place, which is what made them safe to
  read from a render goroutine. What has to outlive a refresh lives in an
  `instanceRuntime` per project and name: the console log drained so far
  (the endpoint hands each byte out once), the `ps` that worked for the
  Top tab, and the newest `Instance` - `Instance.Latest()`, which a ticking
  tab reads so it doesn't show the refresh it opened on forever. The poll
  and an action's own listing overlap and can answer in either order, so
  each listing is numbered before it asks: one that answers late can't
  replace the newest `Instance` with an older one, or drop the runtime of
  an instance a later listing has seen.
- **State changes**: `UpdateInstanceState(name, api.InstanceStatePut{Action:
  ...}, "")` then `op.Wait()`. Stop/restart use a 30s timeout;
  start/freeze/unfreeze use -1. A compose instance goes through each of
  these the way incus-compose puts its own instances through them, since
  its CLI takes only whole services and a replica's row acts on one
  instance (`instance_compose.go`): stop and force stop set
  `user.healthcheck.stopped=true` first, so ic-healthd's restart policy
  leaves it down, and stop falls back to a forced stop unless the clean
  one left it `Stopped` - Incus fails a shutdown that outlives its timeout
  and leaves the instance running, and won't cleanly stop one in `Error`
  at all. When an action fails, the state it left behind decides: stopped
  or frozen keeps the marker (a stop that finished just as its forced stop
  was refused counts as done), still running loses it; incus-compose
  leaves it on either way. Restart is that stop (60s) then a start; start clears
  the marker; freeze sets it, a frozen instance answering no healthcheck,
  and unfreeze clears it. The marker is written with a PATCH of that one
  key, answered synchronously, not a read-modify-write that would race
  ic-healthd's own writes to the same config.
- **Status mid-action**: the daemon has no in-between status for an
  instance. While a stop holds the instance it reports `Running`, and while
  a start does, `Stopped` (`statusCode` in each driver), so a VM reads
  `Running` for the whole of its shutdown. A restart goes from `Running` to
  `Running` with a second or two of `Stopped` between, which the 2-second
  poll rarely lands in, and a row that did would drop to the stopped end
  of the list and back. Restoring a running instance's snapshot is the
  same: Incus stops it, rolls it back and starts it again. So an action
  lazyincus starts marks the instance itself (`Instance.BeginTransition`)
  until a listing taken after it lands: the row reads `starting`,
  `stopping`, `restarting`, `restoring`, `freezing` or `unfreezing`, and
  keeps its place. Pausing is freezing: the API says `freeze`, `Frozen`
  and "Freezing instance", the CLI `incus pause` and `incus resume`.
  lazyincus splits them the way the CLI does, `incus pause` leaving an
  instance `FROZEN` - `p` and the status bar say pause and resume, a row
  freezing, frozen, unfreezing. An action
  anyone else starts gets the same from its operation event (see Events).
  The action and its event both mark the instance, and only the later mark
  can end it, so neither ends the other early.
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
  Drain-on-read only holds for a running container. A VM's read drains
  QEMU's ring buffer into a log file and returns the whole file
  (`ConsoleLog` in the qemu driver), and a stopped instance's is its
  persisted log in full (`instanceConsoleLogGet`), so either replaces the
  buffer rather than adding to it - added, a VM's log repeated itself once
  a second - and a stopped instance's is fetched just once. There's no header-based staleness check available: the client returns
  only `resp.Body` and discards `Last-Modified`.
  `incus-compose logs` reads the same endpoint, so it and the Logs tabs
  drain each other: against 7.4 it printed nothing for running containers
  whose output the tabs had already read. That's why `M` starts empty.
  A VM's console is a serial terminal, and firmware and GRUB write it as
  one: screen clears, cursor moves, `ESC c` resets, and the whole screen
  white-on-black. The tab keeps only the colours (`utils.ConsoleText`),
  less the backgrounds and the black and white that would paint every
  line of a boot grey, and turns a clear into a line break.
- **Deletes the daemon refuses**: a profile named `default`, in any
  project, can't be deleted or renamed - `profileDelete` and
  `profileRename` in `cmd/incusd/profiles.go` refuse the name before
  looking at anything else, confirmed on a 7.4 daemon. A network, a custom
  volume or a profile anything still uses is refused too ("currently in
  use", "still in use"); an image isn't, whatever was created from it. The
  app's prompts go by this - see [docs/Panels.md](Panels.md#confirmations).
- **Network leases**: `GetNetworkLeases` lists only the leases of the
  asking project's instances, and the gateway's addresses only to the
  network's own project. A project without `features.networks` uses
  default's networks, so a compose stack's instances sit on a network that,
  asked from default, has nothing on it but the gateway (seen against
  Incus 7.4). `Network.Leases` asks the network's project and then each
  project its `used_by` URLs name, and merges the answers. A network's
  state carries a bridge's ports as host-side veth names;
  `InstanceStateNetwork.HostName` is the same name from the instance's
  side, which is how the State tab names the instance on each port.
- **Volume usage**: the volume listing carries no sizes; the state
  endpoint does, one volume a request. A `dir` pool without project quotas
  answers with a usage of nothing rather than an error (seen against Incus
  7.4), so a zero is read as unknown, not as empty. An instance's state
  reports each of its disks on such a pool, root and attached volumes
  alike, with a usage of -1.
- **Attach**: `a` shells out to `incus console <name>`, the analog of
  lazydocker's `docker attach`. No detach hint from us - the CLI prints its
  own (`ctrl+a q`) on connect. An OCI application container has no console
  device to attach to (`incus console` fails with "operation not supported
  by device"), so `Instance.IsOCI` - the key behind the type column's
  "(app)" - turns the key into a message pointing at the Logs tab. `E`
  still works there: `incus exec` is a process, not a console. The
  services panel has no `a` at all, a compose service being an OCI image
  as a rule.
- **Exec**: deliberately not the client library's `ExecInstance` websocket
  API — that needs the session's stdio wired into the terminal, which is
  nontrivial to thread through gocui's suspend/resume model (raw mode,
  window-resize messages). `instanceExecShell` shells out to the `incus` CLI
  instead, the same pattern lazydocker uses for `docker exec`. Trade-off:
  requires the `incus` binary on PATH, not just socket access.
