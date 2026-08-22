# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- Daemon connection uses Incus's own remote-resolution logic (`shared/cliconfig`, the same mechanism the `incus` CLI uses) rather than a bare local-unix-socket connect, so it works on non-Linux daemon setups (e.g. `colima start --runtime incus`) where the socket lives at a path recorded in the user's `~/.config/incus/config.yml` remote config instead of a standard Linux location.
- Footer now shows the connected Incus daemon version, remote name, and a live connection indicator (e.g. `Incus v6.11 (colima) ●`), updated from the existing background instance-list poll.
- `Makefile` with `run`, `build`, `test`, `vet`, `lint`, `clean` targets.
- Instances panel's Type column now distinguishes OCI-based application containers from plain containers (`container (app)` vs `container`), matching `incus list`'s TYPE column (checks the `volatile.container.oci` expanded-config key). Populates once background instance-detail refresh has run, same as the IP column.
- Instances panel now shows a snapshot count column, matching `incus list`'s SNAPSHOTS column.
- Main panel Env tab: `KEY=value` list of the instance's environment variables, read from `InstanceFull.ExpandedConfig`'s `environment.*` keys (so profile-inherited variables show up too).

### Changed
- Instances panel's IP addresses column now separates multiple addresses with a space instead of a comma, closer to `incus list`'s layout.

### Fixed
- Footer connection indicator wasn't updating after startup — it was only rendered once (`setInitialViewContent`), so `IsConnected()` changes (e.g. stopping the Incus daemon) never reached the screen. Now redrawn on every background poll tick alongside the instance list refresh.
- Logs tab would show real output right after selecting an instance, then flicker to "Nothing to display" on the next poll tick. Root cause: Incus's console log endpoint drains newly-buffered bytes on each read rather than returning the full accumulated content, so a "replace the display with the latest raw snapshot" polling loop only ever showed whatever arrived in the last second. `Instance.TailConsoleLog()` now accumulates successive reads into a capped per-instance buffer instead. (This also corrects the earlier "Verified" note below, which misdiagnosed empty logs as containers not writing to their console.)

### Verified
- Tested end-to-end against a live Incus daemon (via colima): Instances panel lists real instances with live status/IP, navigation and tab switching work, Config tab renders full instance details correctly.

### Known limitations
- Images, Networks, Volumes, Services/Project panels, custom/bulk commands, the Top tab (per-instance process list) and historical usage graphing, non-English translations, and Windows support are not yet implemented — see [CLAUDE.md](CLAUDE.md).

[Unreleased]: https://github.com/tallica/lazyincus/compare/HEAD...HEAD
