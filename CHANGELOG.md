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

### Known limitations
- Not yet tested against a live Incus daemon.
- Images, Networks, Volumes, Services/Project panels, custom/bulk commands, stats/top tabs, non-English translations, and Windows support are not yet implemented — see [CLAUDE.md](CLAUDE.md).

[Unreleased]: https://github.com/tallica/lazyincus/compare/HEAD...HEAD
