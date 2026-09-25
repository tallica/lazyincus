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
  incus.go                     IncusCommand: connection, project scoping, list/refresh per resource
  instance.go                  Instance: api.InstanceFull as of one refresh; start/stop/restart/freeze/delete/logs/exec
  instance_runtime.go          what outlives a refresh: the drained console log, the working ps, Latest()
  snapshot.go                  Snapshot: create, restore, delete (the list comes with the instances)
  incus_compose.go             compose projects and services, paired with their instances
  compose_config.go            `incus-compose config`: the local project and each service's definition
  incustest/                   a stand-in daemon for tests, answering the listing calls from fixed data
  image.go, network.go, volume.go  the other resources the side panels list
  os.go, os_default_platform.go  subprocess/open-file/open-link helpers (linux/darwin only)
  errors.go, dummies.go        error wrapping; NewDummy* constructors for tests
pkg/gui/
  gui.go                       Gui struct, Run() main loop, background polls, config reload
  refresh.go                   fetch off the main loop, apply on it; the sequence guard between fetches
  tasks_adapter.go             main-panel tasks, and the numbered writes that keep them on the main loop
  side_panels.go               the ordered side panel definitions; number keys, tab cycling
  views.go                     view creation (createAllViews) and styling (styleAllViews)
  layout.go, arrangement.go    boxlayout-driven positioning, including the expand option
  keybindings.go               all key bindings
  focus.go, view_helpers.go    view-stack/focus management, shared render helpers
  *_panel.go                   one per side panel: services, instances, snapshots, images, volumes, networks
  services_actions.go          the services panel's compose verbs and menus
  instance_*.go                per-tab rendering for the instance main panel: info, logs, env, top
  projects.go                  project scope menu (all projects, or one)
  panels/                      generic ListPanel/SideListPanel/FilteredList/ContextState[T]
  presentation/                table-cell rendering, one file per side panel plus menu rows
```

Everything in `pkg/gui` not listed above (`confirmation_panel.go`,
`menu_panel.go`, `options_menu_panel.go`, `filtering.go`, `main_panel.go`,
`app_status_manager.go`, `subprocess.go`, `theme.go`, `window.go`,
`gocui.go`, `panels.go`) is generic gocui plumbing, ported near-verbatim;
`connection.go` is the connection-lost modal.
