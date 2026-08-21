# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial MVP port of [lazydocker](https://github.com/jesseduffield/lazydocker) targeting [Incus](https://linuxcontainers.org/incus/) instead of Docker.
- Instances panel: list containers and VMs, start/stop/restart, pause/unfreeze, delete, view console logs, exec a shell into an instance.
- Main panel Logs and Config tabs for the selected instance.
- YAML user config at `~/.config/lazyincus/config.yml`.
- English-only i18n.
- `LICENSE` (MIT, retaining lazydocker's original copyright as required for this derivative work) and `THIRD_PARTY_NOTICES.md` (gocui, BSD; Incus client, Apache-2.0).
- `.gitignore` for build output and logs.
- `.golangci.yml` lint config, adapted from lazydocker's linter set and migrated to golangci-lint v2's config schema.
- `docs/Config.md` documenting lazyincus's actual (smaller) config schema.
- Daemon connection uses Incus's own remote-resolution logic (`shared/cliconfig`, the same mechanism the `incus` CLI uses) rather than a bare local-unix-socket connect, so it works on non-Linux daemon setups (e.g. `colima start --runtime incus`) where the socket lives at a path recorded in the user's `~/.config/incus/config.yml` remote config instead of a standard Linux location.
- Footer now shows the connected Incus daemon version, remote name, and a live connection indicator (e.g. `Incus v6.11 (colima) ●`), updated from the existing background instance-list poll.
- `Makefile` with `run`, `build`, `test`, `vet`, `lint`, `clean` targets.

### Fixed
- Footer connection indicator wasn't updating after startup — it was only rendered once (`setInitialViewContent`), so `IsConnected()` changes (e.g. stopping the Incus daemon) never reached the screen. Now redrawn on every background poll tick alongside the instance list refresh.

### Verified
- Tested end-to-end against a live Incus daemon (via colima): Instances panel lists real instances with live status/IP, navigation and tab switching work, Config tab renders full instance details correctly. Console log polling returned empty for the OCI-based test containers used — matches the documented caveat about consoles that don't capture init output, not a new bug.

### Known limitations
- Images, Networks, Volumes, Services/Project panels, custom/bulk commands, stats/top tabs, non-English translations, and Windows support are not yet implemented — see [CLAUDE.md](CLAUDE.md).

[Unreleased]: https://github.com/tallica/lazyincus/compare/HEAD...HEAD
