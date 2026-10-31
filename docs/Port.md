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
| `pkg/gui/containers_panel.go` | `pkg/tui/instances.go` |
| `pkg/gui/presentation/containers.go` | `pkg/presentation/instances.go` |
| `~/.config/lazydocker` | `~/.config/lazyincus` (via `xdg.New("", "lazyincus")`) |
| `docker rm` / `docker exec` shell-outs | `incus exec` shell-outs |

`pkg/tasks`, `pkg/log` and `pkg/utils` started as close to line-for-line
ports, none of that code being Docker-specific. lazydocker's `pkg/gui`,
built on gocui, has no counterpart: `pkg/tui` is a Bubble Tea rewrite, so
an upstream bugfix in that layer is a behaviour to check here, not a patch
to carry over.

## Package map

```
main.go                        CLI entry point (flaggy flags, boots pkg/app)
pkg/app/app.go                 wires config -> log -> i18n -> OSCommand -> IncusCommand -> tui
pkg/config/                    YAML user config, defaults, theme; the app's own state.yml and history.yml
pkg/log/                       zerolog setup (console or JSON, file-based when --debug/DEBUG=TRUE)
pkg/i18n/                      TranslationSet; English only
pkg/tasks/                     cancellable background task manager (drives main-panel rendering)
pkg/utils/                     string/table/color/yaml helpers and fuzzy ranking; every display width goes through DisplayWidth
pkg/commands/
  incus.go                     IncusCommand: connection, to any remote by name, project scoping, list/refresh per resource
  instance.go                  Instance: api.InstanceFull as of one refresh; start/stop/restart/freeze/delete/logs/exec
  instance_runtime.go          what outlives a refresh: the drained console log, the working ps, Latest(), a transition mark
  events.go                    the daemon's event stream: one listener, events in the daemon's order
  instance_compose.go          a compose instance's stop/restart/pause, done the way incus-compose does it
  snapshot.go                  Snapshot, an instance's or a custom volume's: create, restore, delete
  incus_compose.go             compose projects and services, paired with their instances
  compose_backup.go            `incus-compose backup`: a stack's backups and what `verify` said of them
  compose_stack.go             ComposeStack, a compose project directory on a remote (remote:dir), and its rolled-up status
  instance_devices.go          what an instance's devices say: published ports, custom volumes, networks
  compose_config.go            `incus-compose config`: a stack's project and each service's definition, and ComposeCmd
  incustest/                   a stand-in daemon for tests, answering the listing calls from fixed data and applying the changes asked of it, and an event stream Emit feeds
  image.go, network.go, volume.go, profile.go  the other resources the side panels list
  image_upstream.go            an OCI image's tag checked against its registry, for the update check
  image_build.go               what an image's config says of its build, and where to read what changed between two
  used_by.go                   which instances use a network, volume, image or profile
  operation.go, warning.go     the daemon's operations and warnings, for `W`'s popup
  completion.go                `incus __complete` for `tab`, and incus-compose's verbs, which it can't be asked for
  os.go, os_default_platform.go  subprocess/open-file/open-link helpers (linux/darwin only)
  errors.go, dummies.go        error wrapping; NewDummy* constructors for tests
pkg/tui/
  tui.go                       Model, Run(), Update's message dispatch
  panels.go                    the ordered side panel definitions; number keys, tab and arrow cycling
  windows.go                   shared slots: the Resources panel's lists as tabs of one window; focus
  layout.go                    boxlayout-driven positioning, including the expand option
  render.go, frame.go, pane.go, popup.go  drawing: list bodies, titled borders, the scrollable main pane, popup placement
  theme.go, mouse.go           the config's colours as escapes; clicks and the wheel
  bindings.go                  all key bindings, their routing, and the read-only guard
  refresh.go, poll.go          fetch off the loop, apply on it; the sequence guard; the pollers
  events.go                    which events refresh what, batching, operation marks, reconnecting
  tasks.go                     main-panel tasks, and the numbered writes that keep them in order
  status.go                    the waiting status and spinner an action shows
  stacks.go, services.go, instances.go, snapshots.go, backups.go, images.go, volumes.go, networks.go, profiles.go  one per side panel, with *_actions.go for its keys
  stack_tabs.go, service_tabs.go  the Stacks and Services panels' main-panel tabs
  snapshot_prompt.go           the snapshot prompt: a name, and options for expiry and stateful
  image_updates.go             the image update check, on the session's remote and every stack's
  image_update_apply.go        the menu of a stack's services with a newer image, to update them
  compose.go, stack_actions.go, service_actions.go  the compose verbs, over a stack or a service
  copy.go                      the `y` menu: what each kind of item offers to copy
  scope.go, remotes.go         the project scope menu and the `R` menu; connecting to the remotes stacks are on, in the background
  daemon_popup.go              the `W` popup: operations.go, the session's operation history, and warnings.go, its lists
  operation_log.go             that history, kept from the event stream
  config_actions.go            `o` and `O`: the config opened, or edited and read again
  palette.go                   `ctrl+p`: every panel's keys and every listed item, from paletteSources
  typed_command.go             `:` and `;`: an incus or incus-compose command typed into a prompt, scoped by the selected row, and its history
  prompt_completion.go         `tab` in those prompts: the word before the cursor, completed off the loop
  prompt.go, filter.go, search.go, menu.go  the one-line text prompt, the list filter, the main-panel search, and menus
  failure_watch.go             a typed or compose command's failure, shown once it ends
  subprocess.go                handing the terminal to `exec`, an editor or a typed command
pkg/presentation/              table-cell rendering, one file per side panel and per `W` list, plus menu rows
```
