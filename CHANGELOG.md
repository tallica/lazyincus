# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `=` expands the focused side panel and collapses it back, so the space the side column gives each list is now a decision you make while reading rather than one you commit to in the config file. `gui.expandFocusedSidePanel` still says which way a session starts, and `=` owns it from then on — a config reload won't pull it back.
- Releases now carry prebuilt binaries for macOS and Linux, on both amd64 and arm64, so installing no longer means having a Go toolchain and building from source. Each release also has a `checksums.txt` to verify the archive you downloaded against.
- lazyincus installs with Homebrew on macOS — `brew install tallica/tap/lazyincus` pours the release binary.

## [0.8.1] - 2026-09-20

### Changed
- The Snapshots panel on a replicated service's own row now lists every replica's snapshots, instead of standing empty because no single instance was selected. The rows group by replica and grow a column naming it, the panel's title carrying the service's name; a replica's own row still shows that replica alone. Taking a snapshot (`n`) from a service's row still picks a replica first, and the panel drops to that one instance so the new snapshot is unambiguous. `d`'s confirmation now names the instance along with the snapshot, replicas being snapshotted under the same names.

## [0.8.0] - 2026-09-20

### Added
- `--remote <name>` (`-r`) picks the daemon for the session, an alternative to `INCUS_REMOTE=<name> lazyincus` and to changing the CLI's default. The flag sets that same variable at startup, so `incus console`, `incus exec` and every incus-compose verb — subprocesses that read the environment themselves — land on the daemon the panels are showing. See [docs/Remotes.md](docs/Remotes.md).
- `--project-directory <dir>` (`-P`) points the Services panel at a compose file somewhere other than the working directory, an alternative to `INCUS_COMPOSE_PROJECT_DIRECTORY`, whose name it borrows and whose variable it sets. A path that isn't a directory is refused at startup rather than showing up as a missing panel.

### Changed
- The Logs tab on a replicated service's own row now shows every replica's log, one after another under the same heading the Info and Config tabs use, instead of a message saying to pick a replica. The streams stay separate rather than interleaved — the console buffers carry nothing to order them by — so `C`'s `logs --follow` is still the merged view of the stack.
- `S`, `s`, `r`, `p`, `d` and `f` act on the selected replica rather than on its whole service: a replica's row is an instance, and these are the instances panel's own keys, so `s` stops that one container and `d` deletes it (incus-compose creates it again on the next `u`). `u`, `U`, `b`, `g` and `C` have no per-replica form and still act on the service, from any of its rows. `x` reads the row under the cursor: `d` reads "bring down" on a service and "delete" on a replica, and the verbs only a service has say so there — "bring up service", not "bring up", which at the top of a replica's menu read as bringing that one replica up. The menu's order follows the row as well: a service's own row leads with the compose verbs as before, a replica's with the instances panel's keys in the instances panel's order.
- A service with replicas now lists them under it in the Services panel, one indented row each, rendered in the same columns as the service. Selecting a replica points everything that needs a single instance at it: the Logs, Env and Top tabs show that replica's rather than a message, Info and Config show that replica alone, the snapshots panel follows it, and `n`, `E` and `y` act on it without first asking which. The service's own row above them means all of its replicas, which is what selecting the service has always meant — every tab reads there exactly as it did before. The compose verbs still act on the whole service from any of its rows, compose having no per-replica form of them.
- Info and Config rule their sections off with the heading inside the rule, rather than printing it as a bare line. Every instance block opens with one, so the rule is reliably where the compose file's half of the tab ends and the daemon's begins. Replicas are numbered — "Replica 2 of 4 · web-2", the blocks being the same labels over and over — and numbered over the service's instances rather than the ones on screen, so a replica's own row reads "2 of 4" while showing only itself. A service with a single instance is headed "Instance · redis-1", no count being worth printing, or just "Instance" where the instance carries the service's own name.
- The Info tab's **Stats** heading is gone, a gap setting the counters off instead: CPU and memory say what they are, and a heading there weighed the same as the ones dividing whole instances, which are a level above it. The counters now line their values up with the identity lines above them, the two halves having run together.
- The Services panel's `ipv4`/`ipv6` columns are blank on a service with replicas: each replica's row carries its own address, and four sets of them side by side pushed the columns after them off the panel.
- The `replicas` column is in the Services panel's default columns, now that a missing replica is a missing row and the figure before the slash is the number of rows underneath. It counts what exists against what the compose file declared, not what's running: the status column says what state the instances are in, and each replica's row says which is in which, so "3/4" means one replica missing rather than one stopped. It stays blank for every service that has what the file asked for, which is every unreplicated one.
- `C` on the Services panel is now the project's actions rather than a list of every verb twice: the same verbs the service keys run, with the `SERVICE` argument left off. It takes no selection, so a stack whose services have never been deployed can be brought up from an all-`none` panel. The verbs it used to hold for the selected service get keys of their own — `p` pauses, or unpauses an already-frozen service, the way the instances panel's `p` toggles freeze; `f` kills (after a confirmation), `b` builds, `g` pulls. The menu lists the same verbs in the same order, pause among them as the one row the key is, plus `logs --follow` for the whole stack.

### Fixed
- A state change or a delete is retried for a few seconds while the daemon reports the instance busy with another operation, instead of failing outright. A compose stack with healthchecks has ic-healthd stamping a verdict into every instance's config on a timer, and a key pressed as one of those landed came back with `Instance is busy running a "update" operation` — most visibly on the force-stop-then-delete path, where the delete follows the stop with nothing in between.
- `a` on an OCI application container now says why instead of failing: those run the image's entrypoint rather than an init system, so there is no console device and `incus console` comes back with "operation not supported by device". The message points at the Logs tab, which is where that output goes, and at `E`, which still works. The Services panel drops `a` altogether, a compose service being an OCI image as a rule.
- Popups no longer grow past the edges of a short terminal. A popup is sized to its content, and one taller than the screen ran off both ends of it rather than scrolling — the keybinding menu (`x`) on anything under about 30 rows. It's now capped to what the screen holds, which is what lets it scroll.

## [0.7.0] - 2026-09-19

### Added
- **Services** side panel, for [incus-compose](https://github.com/lxc/incus-compose) stacks. It appears only when a compose file is in the working directory, and then it's panel `1` — one row per service that file declares, with everything else shifting down a number. Because the rows come from the compose file rather than the daemon, a service that's declared but not running still gets one, in state `none`; the title names the compose project, every row sharing it. Tabs: Info (what the compose file declares for the service — image, command, restart policy, ports, volumes, devices, depends-on — then each of its instances' own Info tab), Logs, Config (the service's slice of `incus-compose config`, then the daemon's own dump for each instance), Env and Top. Logs, Env and Top show the service's one instance, and say so when it has replicas. See [docs/Panels.md](docs/Panels.md#services).
- `u`/`U`/`S`/`s`/`r`/`d` on the Services panel run `incus-compose up`/`up --pull always --recreate`/`start`/`stop`/`restart`/`down` for the selected service, confirmed the way the instances panel's own keys are, and named the way its keys are: bring up, pull & recreate, start, stop, restart, bring down. `C` opens the verbs without a key of their own — `kill`, `pause`, `unpause`, `build`, `pull`, `logs --follow` — each listed for the service and for the whole project.
- `m`/`n`/`a`/`E`/`y` work on the Services panel too, acting on the service's instance and asking which one when it has replicas.
- `gui.serviceColumns` reorders or hides the Services panel's columns, the way `gui.instanceColumns` does for the instances panel and over the same column names, rendered from the service's instances rolled up — plus `replicas`, which is off by default: it's blank unless what's running differs from what the compose file declared.
- `health` and `image` columns in the instances panel, read from incus-compose's config keys and blank for anything created another way. `health` is in the default column set, beside status on both panels; `image` is not. See [docs/Config.md](docs/Config.md).
- README has a [Compose stacks](README.md#compose-stacks) section: what the panel needs, what each key runs, and the columns that read a stack straight from the daemon with no compose file in sight.

### Changed
- The instances panel's **Stats** tab is now **Info**: what the instance is — name, status, type, project, the image it was created from, architecture, created and last-used dates, addresses, snapshot count, and its health where it has one — then those counters under a Stats heading. The Config tab is the YAML dump alone, that identity having moved.
- The snapshots panel follows the Services panel the way it follows the instances panel: the selected service's instance, and none at all while it has replicas.
- `gui.instanceStatusStyle` covers the Services panel's status too, that status being an instance's: a frozen service reads `frozen`, not `stopped`. Only `partial` (replicas disagreeing) and `none` (no instances) are the service's own, with glyphs of their own.
- `ipv6` is no longer one of the instances panel's default columns; `gui.instanceColumns` puts it back.
- The instances panel is titled **Standalone Instances** and leaves out the local stack's instances whenever the Services panel is holding them, mirroring lazydocker. Another project's compose instances stay put — they have no panel of their own.
- Starting lazyincus from a directory with a compose file no longer scopes every panel to that project. The stack has its own panel now, so the instances panel spans projects like the rest; `P` still narrows.

## [0.6.1] - 2026-09-13

### Fixed
- Losing the connection to the daemon crashed the app. It now says so in a modal and keeps polling, closing the modal once the daemon answers again; `esc` dismisses it in the meantime. Nothing reconnects — the client dials per request — so recovery is just the poll getting through.
- No failed action can end the app any more. An error out of a keybinding or a background refresh went straight to gocui's main loop, which exits on it; anything that isn't the daemon being unreachable now opens the usual error panel.
- Requests wait five seconds on the dial, and the connect at startup ten, rather than however long the OS takes to give up on a host that has gone away — which froze the panel for a minute or more. Only the connection is capped; transfers take as long as they take.
- Failing to connect at startup prints the remote and what went wrong with it, instead of a stack trace.

### Added
- [docs/Remotes.md](docs/Remotes.md): connecting to a daemon that isn't the local one — picking a remote with `INCUS_REMOTE`, adding one with a trust token, reaching one over an SSH tunnel, and the three things that go wrong when incusd runs in a local VM. macOS Local Network Privacy and guest clock skew both fail with errors that accuse the wrong component. `scripts/check-local-network.sh` tells the first one apart from a genuinely unreachable host.

## [0.6.0] - 2026-09-12

### Fixed
- A popup could be left stranded on screen: pressing a panel key while a confirmation was open moved focus behind it, with no way back into the dialog. Focus landing outside a prompt now takes the prompt down with it.
- Text typed into a prompt stuck around for the next one, so naming two snapshots in a row produced the two names concatenated. An editable view keeps its contents in a text area that outlives the popup.

### Added
- `n` takes a snapshot from the instances panel as well as the snapshots one, and lands you on the new snapshot afterwards.
- Snapshots can now carry an expiry (after which Incus deletes them) and include the instance's running state. `n` opens a name field with an options box under it — `tab` moves between them, `← →` change the focused field's value, and `enter` or `ctrl+s` creates from either box. Both fields default to off, so name-and-enter is the plain snapshot it always was. Snapshots that expire say so in the panel. Space activates a menu row too, which the binding carried since the port had never actually done.

## [0.5.0] - 2026-09-12

### Added
- All projects are now listed at once, and that's the default: instances, images, volumes and networks from every project, with `P` to scope down to a single one. Actions run against the project the item came from rather than whichever one the client is scoped to, so starting, deleting or snapshotting an instance from another project works from the aggregate view. A project column appears only on panels whose contents actually span projects, so a server with one project looks exactly as it did.
- Optional `service` column for the instances panel, showing the [incus-compose](https://github.com/lxc/incus-compose) service an instance came from (`user.label.incus-compose.service`). Off by default — add it to `gui.instanceColumns` — and blank for instances created any other way.
- `a` attaches to the selected instance's console, shelling out to `incus console` the way `E` does for exec. Detaching is Incus's own ctrl+a q, which it prints on connect.

### Fixed
- The keybinding menu showed nothing for `tab` and a stray `Ė` for `shift+tab`: unnamed keys are rendered by formatting their key code as a character, and those two had no name. They now read `tab` and `shift+tab`.
- The message shown when exec'ing into an instance said to press ctrl-p then ctrl-q to detach — Docker's sequence, carried over by the port. Incus's exec ends when the shell exits, which is what it says now.

## [0.4.0] - 2026-09-12

### Added
- `tab` and `shift+tab` cycle through the side panels, wrapping at both ends, as an alternative to the number keys.
- `gui.expandFocusedSidePanel` gives the focused side panel the space the others aren't using, collapsing them to their title and first row — worth turning on now that there are five panels sharing the side column. Off by default; falls back to an even split on a terminal too short to fit them all collapsed.
- Snapshots panel, showing the selected instance's snapshots and following that selection: `n` takes a new one, `r` restores, `d` deletes, each with a confirmation. This replaces the read-only Snapshots tab in the main panel, which showed the same table with nothing to do to it.
- Images, Volumes and Networks panels, alongside Instances: `1`-`5` switch between them, `d` deletes the selected item after a confirmation, and the main panel shows its details. Side panels split the side column evenly. Only custom volumes and managed networks can be deleted — the rest belong to an instance or to the host, and say so rather than letting the daemon reject the attempt.
- Main panel Top tab: the processes running inside the selected instance, refreshed every two seconds. Incus's API only reports a process count, so this execs `ps` in the instance over the client's websocket exec — instances whose image has no `ps` (many OCI images), and VMs without the Incus guest agent, show the daemon's error instead of a list.

### Changed
- Instances now sort by name, with stopped ones last. Previously each status had its own rank, so a frozen instance sorted between running and stopped ones and the list reshuffled more than it needed to.

### Fixed
- Focusing a panel with nothing in it left the previous panel's content in the main panel: the "no items" message was built as a task and never queued. Most visible on the snapshots panel, which is empty for any instance that has none.
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

[Unreleased]: https://github.com/tallica/lazyincus/compare/v0.8.1...HEAD
[0.8.1]: https://github.com/tallica/lazyincus/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/tallica/lazyincus/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/tallica/lazyincus/compare/v0.6.1...v0.7.0
[0.6.1]: https://github.com/tallica/lazyincus/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/tallica/lazyincus/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/tallica/lazyincus/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/tallica/lazyincus/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/tallica/lazyincus/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/tallica/lazyincus/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/tallica/lazyincus/releases/tag/v0.1.0
