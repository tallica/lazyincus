# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Images panel, the second side panel: `1` and `2` switch between Instances and Images, `d` deletes the selected image after a confirmation, and the main panel shows its details. Side panels split the side column evenly.
- Main panel Top tab: the processes running inside the selected instance, refreshed every two seconds. Incus's API only reports a process count, so this execs `ps` in the instance over the client's websocket exec — instances whose image has no `ps` (many OCI images), and VMs without the Incus guest agent, show the daemon's error instead of a list.

### Changed
- Instances now sort by name, with stopped ones last. Previously each status had its own rank, so a frozen instance sorted between running and stopped ones and the list reshuffled more than it needed to.

### Fixed
- Stopping an instance moved it down the list and left the selection on whatever row it had occupied, so a different instance silently became selected. The cursor now follows the selected instance across a re-sort.

## [0.3.0] - 2026-09-12

### Added
- Project awareness: `P` opens a menu of the server's Incus projects and re-scopes the instance list to the one picked, and the footer now shows which project it's scoped to alongside the remote (e.g. `Incus v6.11 (colima/default) ●`). Instances in any other project were previously invisible — including every `incus-compose` stack, since incus-compose gives each compose project an Incus project of its own.
- `o` opens the lazyincus config file with the configured open command, and `O` opens it in `$VISUAL`/`$EDITOR`. Both are global keybindings.
- [docs/Port.md](docs/Port.md): the lazydocker rename map and package layout, moved out of CLAUDE.md.
- Config changes now take effect without restarting lazyincus — an edit made with `O` as soon as the editor exits, any other edit within ~2s. `gui.screenMode` and `gui.language` still need a restart; see [docs/Config.md](docs/Config.md#reloading).

### Fixed
- `m` opened the Stats tab, not Logs, despite being bound as "view logs" everywhere it's described. It selected main-panel tab 0, which stopped meaning Logs when the Stats tab was added ahead of it. Tabs are now selected by key.
- `gui.screenMode: "full"` silently did nothing: the docs and `config.example.yml` documented `full` while the code only matched `fullscreen`, so `full` fell through to the default. Both spellings are now accepted.

### Changed
- Trimmed the verbose comments carried over from the initial port — the console-log drain-on-read behavior alone was explained in three doc comments plus CLAUDE.md. Code comments now carry what the code can't say; the background lives in CLAUDE.md.
- `docs/Config.md` and `config.example.yml` described `ignore` as matching instance names; it matches any displayed column, status and IP addresses included.
- CLAUDE.md cut from 381 to ~180 lines: the "intentionally dropped" list and the unverified-VM questions now live only in BACKLOG.md (where the work items already were), the rename map and package layout moved to docs/Port.md, and resolved open questions were dropped in favour of the changelog entries that already describe them.

## [0.2.0] - 2026-08-25

### Added
- Deleting a running instance now offers to stop it first and retry, instead of just surfacing Incus's refusal. Mirrors `incus delete --force`: stops without waiting for a clean shutdown, then deletes (skipping the delete for ephemeral instances, which Incus discards on stop).
- Instances panel: `y` copies the selected instance's IPv4 address to the system clipboard (the first address, if it has several). The clipboard tool is auto-detected from `pbcopy`/`wl-copy`/`xclip`/`xsel`, and can be overridden with the new `oS.copyToClipboardCommand` config option.

## [0.1.0] - 2026-08-22

### Added
- Initial MVP port of [lazydocker](https://github.com/jesseduffield/lazydocker) targeting [Incus](https://linuxcontainers.org/incus/) instead of Docker.
- Instances panel: list containers and VMs, start/stop/restart, pause/unfreeze, delete, view console logs, exec a shell into an instance.
- Main panel Logs and Config tabs for the selected instance.
- Main panel Stats tab: CPU (cumulative usage time), memory, process count, per-disk, and per-interface network usage (type, state, host-side veth, MAC, MTU, traffic counters, assigned IP addresses) for the selected instance, read from data the background instance-detail refresh already fetches.
- Main panel Snapshots tab: read-only table of an instance's snapshots (name, taken-at, expires-at, stateful), matching `incus info`'s own Snapshots table.
- YAML user config at `~/.config/lazyincus/config.yml`.
- English-only i18n.
- `LICENSE` (MIT, retaining lazydocker's original copyright as required for this derivative work) and `THIRD_PARTY_NOTICES.md` (gocui, BSD; Incus client, Apache-2.0).
- `.gitignore` for build output and logs.
- `.golangci.yml` lint config, adapted from lazydocker's linter set and migrated to golangci-lint v2's config schema.
- `docs/Config.md` documenting lazyincus's actual (smaller) config schema.
- `config.example.yml`, a fully commented copy of the config defaults to copy from.
- Daemon connection uses Incus's own remote-resolution logic (`shared/cliconfig`, the same mechanism the `incus` CLI uses) rather than a bare local-unix-socket connect, so it works on non-Linux daemon setups (e.g. `colima start --runtime incus`) where the socket lives at a path recorded in the user's `~/.config/incus/config.yml` remote config instead of a standard Linux location.
- Footer now shows the connected Incus daemon version, remote name, and a live connection indicator (e.g. `Incus v6.11 (colima) ●`), updated from the existing background instance-list poll.
- `Makefile` with `run`, `build`, `test`, `vet`, `lint`, `clean` targets.
- Instances panel's Type column now distinguishes OCI-based application containers from plain containers (`container (app)` vs `container`), matching `incus list`'s TYPE column (checks the `volatile.container.oci` expanded-config key). Populates once background instance-detail refresh has run, same as the IP column.
- Instances panel now shows a snapshot count column, matching `incus list`'s SNAPSHOTS column.
- Main panel Env tab: `KEY=value` list of the instance's environment variables, read from `InstanceFull.ExpandedConfig`'s `environment.*` keys (so profile-inherited variables show up too).

### Changed
- Instances panel's IP addresses column now separates multiple addresses with a space instead of a comma, closer to `incus list`'s layout.
- Instances panel now shows name first and status second, instead of status first.
- Instances panel's status column is now lowercased (e.g. `running` instead of `Running`).
- Instances panel's columns (which ones are shown, and in what order) are now configurable via `gui.instanceColumns` (`name`, `status`, `type`, `ipv4`, `ipv6`, `snapshots`). The IP addresses column is now two separate columns, `ipv4` and `ipv6`, instead of one mixed column.
- On macOS, the config file is now read from `~/.config/lazyincus/config.yml` if that directory already exists, before falling back to the previous default of `~/Library/Application Support/lazyincus/config.yml`.

### Fixed
- Footer connection indicator wasn't updating after startup — it was only rendered once (`setInitialViewContent`), so `IsConnected()` changes (e.g. stopping the Incus daemon) never reached the screen. Now redrawn on every background poll tick alongside the instance list refresh.
- Logs tab would show real output right after selecting an instance, then flicker to "Nothing to display" on the next poll tick. Root cause: Incus's console log endpoint drains newly-buffered bytes on each read rather than returning the full accumulated content, so a "replace the display with the latest raw snapshot" polling loop only ever showed whatever arrived in the last second. `Instance.TailConsoleLog()` now accumulates successive reads into a capped per-instance buffer instead. (This also corrects the earlier "Verified" note below, which misdiagnosed empty logs as containers not writing to their console.)
- Logs tab would flood with the same messages repeated every second once an instance was stopped. The drain-on-read behavior `TailConsoleLog` relies on only holds while an instance is running and writing to the live console ring buffer - once stopped, incusd instead serves the same persisted log file on every request, so the accumulate-on-every-fetch logic kept re-appending the whole log every poll tick. `TailConsoleLog` now checks the instance's running state and only fetches once after it stops, leaving the buffer untouched until it starts running again.
- `config.example.yml`'s `oS.openCommand`/`openLinkCommand` were shown set to literal empty strings; copying the file verbatim would have silently overridden the platform-computed default (e.g. macOS's `open {{filename}}`) with an empty command, breaking file/link opening. Both are now commented out with an explanation instead.

### Verified
- Tested end-to-end against a live Incus daemon (via colima): Instances panel lists real instances with live status/IP, navigation and tab switching work, Config tab renders full instance details correctly.

### Known limitations
- Images, Networks, Volumes, Services/Project panels, custom/bulk commands, the Top tab (per-instance process list) and historical usage graphing, non-English translations, and Windows support are not yet implemented — see [BACKLOG.md](BACKLOG.md).
- VM instances are untested beyond basic listing/start/stop/delete: freeze/unfreeze, exec, and delete-while-running haven't been verified against a real VM (only containers so far) — see [BACKLOG.md](BACKLOG.md#blocked).

[Unreleased]: https://github.com/tallica/lazyincus/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/tallica/lazyincus/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/tallica/lazyincus/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/tallica/lazyincus/releases/tag/v0.1.0
