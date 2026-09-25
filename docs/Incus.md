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
  at all. An action that fails takes the marker off again, since the
  instance is still up; incus-compose leaves it on. Restart is that stop (60s) then a start; start clears
  the marker; freeze sets it, a frozen instance answering no healthcheck,
  and unfreeze clears it. The marker is written with a PATCH of that one
  key, answered synchronously, not a read-modify-write that would race
  ic-healthd's own writes to the same config.
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
