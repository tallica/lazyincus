# lazyincus

A terminal UI for [Incus](https://linuxcontainers.org/incus/) (the system
container/VM manager, LXD fork), ported from
[lazydocker](https://github.com/jesseduffield/lazydocker) — a terminal UI for
Docker built on [gocui](https://github.com/jesseduffield/gocui).

This file documents the port itself: what was carried over, what was
rewritten, what was dropped, and the decisions/assumptions made along the way
(some of which are still unverified against a live Incus daemon — see "Open
questions" at the bottom).

## Status

MVP. Builds clean (`go build ./...`, `go vet ./...`), has a small unit test
suite (`go test ./...`), and has been run end-to-end against a real Incus
daemon (via `colima start --runtime incus` on macOS): the Instances panel
listed real containers with live status/IP, navigation and tab switching
worked, and the Config tab rendered full instance details correctly. Some
paths remain untested since only containers (no VMs) were available — see
"Open questions" below.

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
grafting histories would imply a closer lineage than actually exists. This
commit pin is the substitute: the way to check later whether a bugfix that
landed in lazydocker's shared TUI/config plumbing after this date also
applies here.

Naming conventions used throughout:

| lazydocker | lazyincus |
|---|---|
| `Docker` / `docker` | `Incus` / `incus` |
| `Container` | `Instance` |
| `DockerCommand` | `IncusCommand` |
| `pkg/commands/docker.go` | `pkg/commands/incus.go` |
| `pkg/commands/container.go` | `pkg/commands/instance.go` |
| `pkg/gui/containers_panel.go` | `pkg/gui/instances_panel.go` |
| `pkg/gui/container_logs.go` | `pkg/gui/instance_logs.go` |
| `pkg/gui/presentation/containers.go` | `pkg/gui/presentation/instances.go` |
| `~/.config/lazydocker` | `~/.config/lazyincus` (via `xdg.New("", "lazyincus")`) |
| `docker rm` / `docker exec` shell-outs | `incus exec` shell-outs |

Framework/plumbing packages (`pkg/tasks`, `pkg/log`, `pkg/utils`, the generic
half of `pkg/gui`, the generic list-panel machinery in `pkg/gui/panels`) are
close to line-for-line ports — only import paths and branding strings
changed, since none of that code is Docker-specific.

## Package map

```
main.go                        CLI entry point (flaggy flags, boots pkg/app)
pkg/app/app.go                 wires config -> log -> i18n -> OSCommand -> IncusCommand -> Gui
pkg/config/                    YAML user config (~/.config/lazyincus/config.yml), defaults, theme
pkg/log/                       logrus setup (JSON, file-based when --debug/DEBUG=TRUE)
pkg/i18n/                      TranslationSet; English only for this MVP
pkg/tasks/                     cancellable background task manager (drives main-panel rendering)
pkg/utils/                     string/table/color/yaml helpers, unchanged from lazydocker
pkg/commands/
  incus.go                     IncusCommand: connects via cliconfig.LoadConfig + GetInstanceServer, lists/refreshes instances
  instance.go                  Instance: wraps api.Instance, start/stop/restart/freeze/delete/logs
  os.go, os_default_platform.go  subprocess/open-file/open-link helpers (linux/darwin only; no windows)
  errors.go                    WrapError (go-errors stack-trace wrapping)
  dummies.go                   NewDummy* constructors for tests, mirrors lazydocker/pkg/commands/dummies.go
pkg/gui/
  gui.go                       Gui struct, NewGui, Run() main loop, background refresh polling
  views.go                     gocui view creation/titles/styling
  layout.go, arrangement.go    boxlayout-driven panel positioning (single side panel: instances)
  keybindings.go               all key bindings (global + instances panel + main panel + menu/filter)
  focus.go, view_helpers.go    view-stack/focus management, shared render helpers
  instances_panel.go           the Instances side panel: list, sort, filter, start/stop/restart/
                                pause-freeze/delete/logs/exec handlers
  instance_logs.go             console-log polling into the main panel + promptToReturn()
  instance_stats.go            Stats tab rendering
  instance_env.go              Env tab rendering
  instance_snapshots.go        Snapshots tab rendering
  confirmation_panel.go, menu_panel.go, options_menu_panel.go, filtering.go, main_panel.go,
  app_status_manager.go, tasks_adapter.go, subprocess.go, theme.go, window.go, gocui.go, panels.go
                                generic gocui plumbing, ported near-verbatim from lazydocker
  panels/                      generic ListPanel/SideListPanel/FilteredList/ContextState[T] machinery
  presentation/instances.go    table-cell rendering + status coloring for the instances list
  presentation/menu_items.go   menu row rendering
  types/types.go                MenuItem
```

## The one panel: Instances

The only panel in this MVP. Lists both containers and VMs
(`GetInstances(api.InstanceTypeAny)`), columns roughly mirroring `incus
list`: status, name, type (`vm` / `container` / `container (app)` for
OCI-based application containers, via the `volatile.container.oci`
expanded-config key), IP addresses (from `InstanceFull.State.Network`,
global-scope addresses only, space-separated - not comma-separated, to
match `incus list`'s own per-column layout more closely), and snapshot
count (`len(InstanceFull.Snapshots)`). The `(app)` suffix, IP addresses,
and snapshot count all only appear once `RefreshInstanceDetails` has
fetched full details in the background (showing bare `container`/`vm`
and `0` snapshots until then).

Keybindings are listed in [README.md](README.md#usage) (the canonical
source — keep that table current when keybindings change, not this file).
Implementation details the README table doesn't cover:
- `s` (Stop) and `d` (Delete) show a confirmation panel before acting.
- `p` (Pause) toggles between `freeze` and `unfreeze` based on the
  instance's current status, rather than being a single fixed action.

Main panel has five tabs (down from lazydocker's five — logs/stats/env/config/top,
with top dropped and Env re-added later once instance state fetching was in
place; roughly mirrors what `incus info <name>` prints in one shot, split
across tabs instead):
- **Stats** — CPU/memory/process count/disk/per-interface network usage
  (type, state, host-side veth name, MAC, MTU, traffic counters, assigned
  IP addresses with family/scope), read from `InstanceFull.State` (already
  kept current by `RefreshInstanceDetails`'s background poll, so no extra
  API calls needed) and re-rendered every second. CPU is shown as
  cumulative usage time, not a percentage — Incus's API reports total
  nanoseconds consumed since start, not an instantaneous rate, matching
  what `incus info <name>` itself shows.
- **Logs** — polls `Instance.TailConsoleLog()` every second and re-renders
  the accumulated buffer (see "Incus client integration details" above for
  why a raw per-poll snapshot doesn't work).
- **Config** — YAML dump of `api.InstanceFull` (name/type/status/created/profiles
  header, then the full struct via `utils.MarshalIntoYaml`).
- **Env** — static (non-polling) `KEY=value` list of the instance's
  environment variables, read from `InstanceFull.ExpandedConfig` entries
  prefixed `environment.` (Incus's own env-var config key, e.g. `incus
  config set <name> environment.FOO=bar` — see `doc/instance-exec.md` in
  the Incus source). Read from the *expanded* config rather than the raw
  per-instance one so profile-inherited variables show up too, not just
  ones set directly on the instance.
- **Snapshots** — static (non-polling) table of `InstanceFull.Snapshots`:
  name, taken-at, expires-at, stateful — matches `incus info`'s own
  Snapshots table exactly, including the *absence* of a size column
  (`InstanceSnapshot.Size` comes back as `-1`/unset from `GetInstanceFull`;
  a real size would need a separate per-snapshot request we're not making).
  Read-only for now: no create/restore/delete actions, since those need
  per-row selection and keybindings that don't fit this plain-text
  main-panel tab model — a real interactive Snapshots panel would need to
  be a second **side panel** (like lazydocker's Images/Volumes/Networks),
  not a main-panel tab, since the app currently only has one side panel
  (Instances) and no panel-switching infrastructure yet.

## Incus client integration details

- **Connection**: `cliconfig.LoadConfig("")` + `cliCfg.GetInstanceServer(cliCfg.DefaultRemote)`
  in `pkg/commands/incus.go`, using Incus's own `shared/cliconfig` package —
  the same remote-resolution logic the `incus` CLI itself uses. It loads
  `~/.config/incus/config.yml` (or the platform equivalent, or `$INCUS_CONF`)
  and connects to whichever remote is marked as `default-remote` there,
  handling unix-socket and TLS-authenticated remotes alike. This turned out
  to matter beyond native Linux hosts: on setups where the daemon runs
  inside a VM (e.g. `colima start --runtime incus` on macOS), the "local"
  socket lives at a path recorded in that remote's config
  (`unix:///Users/you/.colima/default/incus.sock`), not at any of the
  standard Linux socket locations — so Incus *does* have a per-user
  remote-context system analogous to lazydocker's `determineDockerHost()` /
  docker-context / `DOCKER_HOST` dance, it just isn't needed on a native
  Linux host talking to its own local daemon directly.
- **List instances**: `Client.GetInstances(api.InstanceTypeAny)` returns
  `[]api.Instance` (name, status, status code, type, created-at). Existing
  `*Instance` objects are matched by name and reused across refreshes so any
  cached `full` details survive.
- **Full details / state**: `Client.GetInstanceFull(name)` → `*api.InstanceFull`
  (embeds `api.Instance` + `*api.InstanceState` with CPU/memory/network/disk
  usage). Fetched in the background every second per instance
  (`IncusCommand.RefreshInstanceDetails`), not on every list refresh.
- **State changes**: `Client.UpdateInstanceState(name, api.InstanceStatePut{Action: ...}, "")`
  then `op.Wait()`. Actions used: `start`, `stop`, `restart`, `freeze`,
  `unfreeze`. Stop/restart use a 30s timeout; start/freeze/unfreeze use -1
  (no timeout).
- **Delete**: `Client.DeleteInstance(name)` + `op.Wait()`. Incus refuses to
  delete a running instance; this MVP does **not** replicate lazydocker's
  "force stop and retry" flow (`ComplexError`/`MustStopContainer`) — a
  failed delete just surfaces the raw Incus error in the confirmation
  panel. The user has to stop the instance first.
- **Logs**: `Client.GetInstanceConsoleLog(name, &incus.InstanceConsoleLogArgs{})`
  returns an `io.ReadCloser` over the console ring buffer. This is
  fundamentally different from Docker's `ContainerLogs(..., Follow: true)`
  streaming API — Incus's console log is pull-based, not a stream. It's
  also **drain-on-read**: despite `incusd`'s handler requesting
  `ClearLog: false` from liblxc (`cmd/incusd/instance_console.go`), each
  successful read only returns bytes buffered since the *previous* read —
  confirmed directly against the `incus` CLI, not just this client:
  `incus console <name> --show-log` run twice in a row on a busy service
  (nginx) shows real startup output on the first call and nothing on the
  second, despite the instance still running and producing output.
  `Instance.ConsoleLog()` exposes that raw one-shot behavior; `Instance.
  TailConsoleLog()` accumulates successive reads into a capped
  (256 KiB) in-memory buffer per instance, and `renderInstanceLogsToMain`
  polls *that* once a second — a naive "replace displayed content with
  each raw snapshot" loop (the original implementation) flickers to
  "Nothing to display" on every tick where nothing new was buffered.
  The drain-on-read behavior itself only holds while the instance is
  running: incusd reads the live console ring buffer in that case, but
  once stopped it instead serves the persisted log file as-is on every
  request (see `instanceConsoleLogGet` in `cmd/incusd/instance_console.go`)
  - the same content back every time, not fresh bytes. Blindly re-fetching
  and re-appending every poll tick would flood the buffer with duplicate
  messages once stopped, so `TailConsoleLog` checks `IsRunning()` and only
  fetches once after the instance stops (to pick up any final output),
  leaving the buffer alone until it starts running again. There's no
  response-header-based way to detect staleness instead (e.g.
  `Last-Modified`, which the daemon's `FileResponse` does set for the
  stopped-instance path) - `GetInstanceConsoleLog` in the client library
  returns only `resp.Body`, discarding the rest of the response.
- **Exec**: deliberately **not** implemented via the client library's
  `ExecInstance`/websocket API. That call needs the exec session's stdio
  wired directly into the terminal, which the client library exposes via
  `InstanceExecArgs{Stdin, Stdout, Stderr}` — doable, but nontrivial to
  thread through gocui's suspend/resume subprocess model correctly (raw
  terminal mode, window-resize control messages, etc.) without a live
  daemon to test against. Instead, `instanceExecShell` shells out to the
  `incus` CLI (`incus exec <name> -- sh -c 'exec $(command -v bash || ...)'`)
  via `gui.runSubprocessWithMessage`, the same pattern lazydocker itself
  uses for `docker exec`/`docker attach`. Trade-off: requires the `incus`
  CLI binary on PATH in addition to socket access.

## What was intentionally dropped for this MVP

Per the brief, these lazydocker panels/features were **not** ported:

- Images, Networks, Volumes panels
- Services / Project panels (docker-compose equivalent — Incus has no
  compose-like grouping concept to map this onto anyway)
- Custom commands system (`c` key, `config.CustomCommands`)
- Bulk commands system (`b` key, `config.BulkCommands`)
- Top tab (per-instance process list) and the whole `ContainerStats`/
  `RecordedStats` historical monitoring machinery (`CreateClientStatMonitor`,
  CPU/mem history over time, graphing config). A point-in-time Stats tab
  (current CPU/mem/disk/network usage, no history/graphing) was added
  later - see CHANGELOG.md.
- Non-English translations (only `pkg/i18n/english.go` was ported;
  `NewTranslationSetFromConfig` currently just returns the English set
  regardless of the configured language)
- `pkg/commands/ssh` (docker-over-SSH tunneling — not applicable to a local
  Incus unix socket)
- Windows support (`config_windows.go`, `os_windows.go`,
  `docker_host_windows.go` equivalents were not ported; the app currently
  only builds `!windows` platform files)
- Docker's daemon event stream (`/events`) had no use here — replaced with
  a plain 2-second polling refresh (`gui.goEvery(time.Second*2,
  gui.refreshInstancesQuiet)`) since Incus's client library doesn't expose
  an events feed that was worth wiring up for this pass.

## Config

`pkg/config/app_config.go` keeps `GuiConfig` (scroll/theme/border/screen-mode/
side-panel-width/etc, renamed `ContainerStatusHealthStyle` →
`InstanceStatusStyle`), `ConfirmOnQuit`, `OSConfig` (open-file/open-link
commands), and `Ignore`. Dropped entirely: `CommandTemplatesConfig` (all
docker-compose command templates), `CustomCommands`, `BulkCommands`,
`StatsConfig`/`GraphConfig`, `Replacements`, `LogsConfig` (Since/Tail/
Timestamps don't map onto Incus's console-log snapshot model).

Config file lives at `~/.config/lazyincus/config.yml` (via
`xdg.New("", "lazyincus")`, same mechanism lazydocker uses minus the legacy
`jesseduffield`-vendor fallback path).

## Open questions / unverified assumptions

No Incus daemon was available while building the initial port, so several
assumptions were plausible-but-unverified at first. It has since been run
end-to-end against a real daemon (via `colima start --runtime incus`, two
OCI-based containers: `alpine`, `nginx`) — see items below for what that
did and didn't confirm.

1. ~~Console log for containers without a capturing init~~ — **resolved**,
   and the original theory was wrong. It wasn't that alpine/nginx weren't
   writing to their console; it's that the console log endpoint drains on
   read (see "Incus client integration details" above), so by the time a
   1s-later poll asked again, there was nothing *new* since the previous
   read. Fixed by accumulating reads client-side (`TailConsoleLog`)
   instead of treating each snapshot as the full log.
2. **Freeze/unfreeze on VMs** — still untested; only containers were
   available. `freeze`/`unfreeze` are documented as container-oriented
   actions. The Pause key (`p`) does not check `instance.IsVM()` before
   offering it; behavior against a VM (error vs. silent no-op vs. actual
   suspend) is unconfirmed.
3. **Exec into a VM** — still untested; only containers were available.
   `incus exec` requires the Incus guest agent to be running inside the
   VM; if it isn't, the exec shell-out will simply fail with whatever
   error the `incus` CLI prints. No special-casing was added.
4. **Delete-while-running UX** — still untested (didn't delete the test
   containers). A failed delete just shows the raw API error rather than
   offering a force-stop-then-delete flow. Worth revisiting once real
   error text from `DeleteInstance` on a running instance is known.
5. **IP address column** — confirmed working: both test containers showed
   correct IPv4/IPv6 addresses (`InstanceFull.State.Network`, filtered to
   `scope == "global"`, excluding `lo`), populated a second or so after
   startup once the background refresh (`RefreshInstanceDetails`) ran.

## Building / testing

```sh
cd /Volumes/Projects/tallica/lazyincus
go build ./...
go vet ./...
go test ./...
```

No `vendor/` directory — plain module mode.

For the history of what's been added/changed since the initial port, see
[CHANGELOG.md](CHANGELOG.md) and `git log` — this file documents the port's
current architecture, not its change history, to avoid the two drifting out
of sync with each other.
