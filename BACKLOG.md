# Backlog

Ideas and follow-up work not yet scheduled. Not a roadmap or a commitment —
just a place to park things so they don't get lost. See
[CHANGELOG.md](CHANGELOG.md) for what's actually shipped and
[CLAUDE.md](CLAUDE.md) for why the current MVP is scoped the way it is.

Actionable items are checkboxes — tick them as they land, and move the real
description of what shipped into CHANGELOG.md. Plain bullets are context or
decisions, not work: they'll never be ticked.

## How the lazydocker gaps were established

The comparison below was made against lazydocker at the commit this port
pins ([`7e7aadc`](https://github.com/jesseduffield/lazydocker/commit/7e7aadc2071d58031bf2daafca1fbd4093efc23f)),
reading its `pkg/gui` file list and its generated
`docs/keybindings/Keybindings_en.md` cheatsheet, then diffing that against
what's actually wired up in `pkg/gui/keybindings.go` here. Re-run that
comparison if the pin ever moves.

## Panel-switching infrastructure

Shipped. `sidePanelDefs()` is the ordered list every part of the side-panel
machinery derives from — see CLAUDE.md; the staged plan that got there is in
the git history. Five panels, number keys, `tab`/`shift+tab` cycling, and
`gui.expandFocusedSidePanel` for when an even split is too cramped.

## Missing vs lazydocker

### Side panels

lazydocker has six side panels; lazyincus has five.

- [ ] **An "about"/credits surface** — lazydocker's Project panel hosted its
      credits tab, so dropping that panel left lazyincus with nowhere to put
      one (`CreditsTitle` is ported but unused).

Not planned as lazydocker has them:

- **Services / Project panels** — these were docker-compose specific, and the
  port assumed Incus had nothing to map them onto. That assumption is now
  wrong: see [incus-compose integration](#incus-compose-integration).

### Per-instance actions

- [x] **Attach (`a`)** — `incus console <name>`. VMs can also take
      `--type vga` for a graphical console; not offered, since it opens a
      separate viewer rather than using the terminal.
- [ ] **Open in browser (`w`)** — lazydocker opens the container's first HTTP
      port. Incus has no port-mapping concept, but "open `http://<ipv4>`" is
      the obvious translation, and `OSCommand.OpenLink` already exists (the
      footer's donate link uses it).

Not planned:

- **Custom commands (`c`) / bulk commands (`b`)** — dropped by design, along
  with their config sections.

### Global

- [x] **Edit/open config (`e` / `o`)** — bound globally as `o` (open) and
      `O` (edit).
- [ ] **Cheatsheet generator** — lazydocker generates `docs/keybindings/*.md`
      from its i18n set via `scripts/cheatsheet`. Here the README keybinding
      table is hand-maintained, which is why CLAUDE.md has to carry a
      reminder to keep it current.

Not planned:

- **Stats history / graphing** — the Stats tab is point-in-time only;
  lazydocker's `RecordedStats`/graph config machinery wasn't ported.
- **Non-English translations**, **Windows support**.
- **Event stream** — lazydocker consumes Docker's `/events`; lazyincus polls
  every 2s instead. Fine in practice; noted only for completeness.

### Cleanup

- [ ] **Unused translation strings** — eleven are defined but never
      referenced: `MainTitle`, `GlobalTitle`, `ErrorOccurred`,
      `ConnectionFailed`, `ForceRemove`, `NoInstance`, `RemoveWithForce`,
      `FilterList`, `SortInstancesByState`, `CreditsTitle`,
      `CannotDisplayEnvVariables`. Dead weight now that attach is wired up.
      Wire up or delete.

## Not lazydocker-shaped

The interesting gaps aren't all inherited from lazydocker. Profiles,
projects, and remote-switching are Incus concepts with no Docker analog, so
they never show up in the comparison above and are worth considering on their
own merits.

- [ ] **Copy menu** — generalize `y` (currently hardcoded to first IPv4) into
      a menu: name / IPv4 / IPv6 / all addresses. The `Instance.Addresses`
      and `OSCommand.CopyToClipboard` plumbing is already generic; this is
      mostly a menu panel plus entries.
- [ ] **Profiles / remotes** — no panels or switching UI for either. The
      daemon connection uses whichever remote is `default-remote` in the
      user's Incus config and never offers to change it. (Projects now have
      a switcher — see [incus-compose integration](#incus-compose-integration).)

Deliberately deferred (don't re-pitch unprompted):

- **Instance creation** — considered and skipped. Creation is a rare one-off
  usually done from the CLI, and the expensive part isn't the API call but
  the missing multi-field input UI (the repo has only a single-line prompt
  panel), plus an image browser would need a separate image-server
  connection (`GetImageServer` against the `images:` simplestreams remote,
  distinct from the instance server). If revived: build the thin version
  first — prompt for one string and shell out to `incus launch` via
  `runSubprocessWithMessage`, mirroring `instanceExecShell`. That also shows
  image-download progress natively, which a `WithWaitingStatus` spinner would
  hide.

## incus-compose integration

[incus-compose](https://github.com/lxc/incus-compose) is a drop-in
replacement for `docker compose` that runs an unmodified `compose.yaml`
against Incus, pulling OCI images straight from docker.io/ghcr.io via Incus's
native OCI support. It started as [bketelsen/incus-compose] and now lives
under the LXC org; its docs call it stable, and it needs Incus 7.0.1 LTS or
7.2+ (this port builds against client v7.3.0, so no version problem).
Commands mirror compose: `up`, `down`, `start`, `stop`, `restart`,
`list`/`ps`, `logs`, `exec`, `config`, `build`.

[bketelsen/incus-compose]: https://github.com/bketelsen/incus-compose

This is the missing piece that made lazydocker's Services/Project panels look
unportable. It matters to lazyincus in two independent ways.

### 1. Project awareness (shipped)

**incus-compose creates one Incus project per compose project** — run
`incus-compose -p myapp up` and you get an Incus project `myapp` holding that
stack's instances, networks and volumes, plus a separate
`incus-compose-cache` project for pulled images.

That's why project awareness came first: without it a compose stack was
invisible unless the user pointed their `incus` CLI remote at its project.
Every project is listed by default now, `P` scopes to one, and the footer
says which — see CLAUDE.md. Nothing outstanding here.

### 2. Compose grouping on top

Each instance incus-compose creates is stamped with config keys naming its
origin, read from `project/instance.go` in the source:

| Key | Holds |
|---|---|
| `user.label.incus-compose.project` | compose project name |
| `user.label.incus-compose.service` | service name the instance came from |
| `user.label.<compose label>` | each of the service's own compose labels |
| `user.healthcheck.enabled` / `user.healthcheck.*` | healthcheck opt-in and its test/interval/retries, driven by an `ic-healthd` sidecar |
| `user.incus-compose.oneoff` | marks a one-off (`run`-style) instance |
| `environment.<KEY>` | the service's environment variables |

Instances are named `<service>-<index>` (`web-1`, `app-1`).

Two things fall out of this. Those keys live in `ExpandedConfig`, which
`RefreshInstanceDetails` already fetches — so both are presentation work on
data lazyincus has in hand, not new API calls.

- [x] **Service column** — opt-in `service` column reading
      `user.label.incus-compose.service`.
- [ ] **Grouping by service** — the other half of what lazydocker's Services
      panel gave you. Needs a think about how grouping fits a flat
      SideListPanel.
- [ ] **Health column** — `user.healthcheck.enabled` plus the healthd state
      would give lazyincus a health indicator, which the port dropped along
      with Docker's healthcheck support. Needs a look at where `ic-healthd`
      writes results before this is more than a guess.
- [ ] **`incus-compose` shell-outs** — `up`/`down`/`restart` on the selected
      project, in the `instanceExecShell` subprocess style. Optional, and it
      adds a second CLI dependency beyond `incus` itself.

### Caveats

- The key names above were read out of `project/instance.go` on GitHub and
  re-checked against `main` when the service column landed, but never
  observed on a live incus-compose setup — the column was tested by setting
  the label by hand. Worth confirming against a real stack.
- Those keys are internal to incus-compose and carry no compatibility
  promise. If lazyincus depends on them, pin the commit they were read from
  the way the lazydocker port pin works, so a drift has somewhere to be
  checked against.

## Snapshots panel

Shipped as a side panel following the instances panel's selection, with
create/restore/delete; the read-only main-panel tab it replaced is gone.
What's left:

- [x] Stateful snapshots and expiry — both are fields in the `n` popup,
      cycled with `← →`. The daemon's own refusal explains what stateful
      wants beyond a running instance (`migration.stateful` on it).
- [ ] Custom expiry — the field cycles never, 1, 7 and 30 days; anything
      else needs the CLI. A free-text duration would want parsing and an
      error path, or a `custom…` value that opens a prompt.
- [ ] Rename (`RenameInstanceSnapshot`), if it turns out to be wanted.

## Housekeeping

- [x] **Backfill the GitHub releases for v0.1.0 and v0.2.0** — both cut
      from their changelog sections, flagged not-latest so v0.3.0 keeps that
      badge. Every tag now has a release.
- [ ] **Attach built binaries to releases** — releases currently carry
      notes only, so installing still means `go build` from source. Needs a
      decision on which platforms to build for (linux/amd64 and
      linux/arm64 at minimum) and whether that runs in CI.

## Blocked

Everything here needs a real VM instance, so it needs a host that can
provide one: a Linux machine running Incus directly, or — when the daemon
runs inside a VM, as under colima on macOS — hardware nested virtualization,
which on Apple Silicon means an M3 or later with macOS 15+. Without
`/dev/kvm` inside the guest, `incus launch ... --vm` fails with `KVM support
is missing (no /dev/kvm)` and no colima or Incus flag substitutes for it.

- [ ] Verify freeze/unfreeze against a real VM. Both are documented as
      container-oriented actions, and `p` doesn't check `IsVM()` before
      offering them — error, silent no-op or actual suspend is unconfirmed.
- [ ] Verify exec into a real VM. `incus exec` needs the Incus guest agent
      running inside the VM; without it the shell-out fails with whatever
      the CLI prints. No special-casing was added.
