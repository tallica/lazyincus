# Porting reference

Lookup material for working against lazydocker's source — the rename map and
the package layout. See [CLAUDE.md](../CLAUDE.md) for the architecture and
the upstream commit this port pins.

## Naming conventions

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

`pkg/tasks`, `pkg/log`, `pkg/utils`, the generic half of `pkg/gui` and the
list-panel machinery in `pkg/gui/panels` started as close to line-for-line
ports, none of that code being Docker-specific. They've since moved apart
where lazydocker's design raced its own main loop — refreshes and
main-panel tasks now hand every view change to the loop (see CLAUDE.md's
Panels section) — and gocui is on master rather than lazydocker's 2024 pin,
so an upstream bugfix wants checking against both before it's copied.

## Package map

```
main.go                        CLI entry point (flaggy flags, boots pkg/app)
pkg/app/app.go                 wires config -> log -> i18n -> OSCommand -> IncusCommand -> Gui
pkg/config/                    YAML user config, defaults, theme
pkg/log/                       logrus setup (JSON, file-based when --debug/DEBUG=TRUE)
pkg/i18n/                      TranslationSet; English only
pkg/tasks/                     cancellable background task manager (drives main-panel rendering)
pkg/utils/                     string/table/color/yaml helpers; every display width goes through DisplayWidth
pkg/commands/
  incus.go                     IncusCommand: connection, to any remote by name, project scoping, list/refresh per resource
  instance.go                  Instance: api.InstanceFull as of one refresh; start/stop/restart/freeze/delete/logs/exec
  instance_runtime.go          what outlives a refresh: the drained console log, the working ps, Latest(), a transition mark
  events.go                    the daemon's event stream: one listener, events in the daemon's order
  instance_compose.go          a compose instance's stop/restart/pause, done the way incus-compose does it
  snapshot.go                  Snapshot, an instance's or a custom volume's: create, restore, delete
  incus_compose.go             compose projects and services, paired with their instances
  compose_stack.go             ComposeStack, a compose project directory on a remote (remote:dir), and its rolled-up status
  instance_devices.go          what an instance's devices say: published ports, custom volumes, networks
  compose_config.go            `incus-compose config`: a stack's project and each service's definition, and ComposeCmd
  incustest/                   a stand-in daemon for tests, answering the listing calls from fixed data, and an event stream Emit feeds
  image.go, network.go, volume.go, profile.go  the other resources the side panels list
  used_by.go                   which instances use a network, volume, image or profile
  os.go, os_default_platform.go  subprocess/open-file/open-link helpers (linux/darwin only)
  errors.go, dummies.go        error wrapping; NewDummy* constructors for tests
pkg/gui/
  gui.go                       Gui struct, Run() main loop, background polls, config reload
  refresh.go                   fetch off the main loop, apply on it; the sequence guard between fetches
  events.go                    which events refresh what, batching, operation marks, reconnecting
  tasks_adapter.go             main-panel tasks, and the numbered writes that keep them on the main loop
  side_panels.go               the ordered side panel definitions; number keys, tab and arrow cycling
  window.go                    shared slots: the Resources panel's lists as tabs of one window
  views.go                     view creation (createAllViews) and styling (styleAllViews)
  layout.go, arrangement.go    boxlayout-driven positioning, including the expand option
  keybindings.go               all key bindings
  focus.go, view_helpers.go    view-stack/focus management, shared render helpers
  *_panel.go                   one per side panel: stacks, services, instances, snapshots, images, volumes, networks, profiles
  stacks_actions.go            the Stacks panel's compose verbs, over the whole stack
  stack_info.go                the stack's Info tab: identity, endpoints, usage, drift
  copy.go                      the `y` menu: what each kind of item offers to copy
  services_actions.go          the services panel's compose verbs
  instance_*.go                per-tab rendering for the instance main panel: info, logs, env, top
  projects.go                  project scope menu (all projects, or one), and the reload after any change of scope
  remotes.go                   the `R` menu and switching remote; connecting to the remotes stacks are on, in the background
  panels/                      generic ListPanel/SideListPanel/FilteredList/ContextState[T]
  presentation/                table-cell rendering, one file per side panel plus menu rows
```

Everything in `pkg/gui` not listed above (`confirmation_panel.go`,
`menu_panel.go`, `options_menu_panel.go`, `filtering.go`, `main_panel.go`,
`app_status_manager.go`, `subprocess.go`, `theme.go`, `gocui.go`,
`panels.go`) is generic gocui plumbing, ported near-verbatim;
`connection.go` is the connection-lost modal, and `prompt_panel.go` the
one-line text prompt Stacks' `a` and `e` ask for a directory with.
