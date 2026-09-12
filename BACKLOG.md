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

The recurring blocker, and the highest-leverage work outstanding. The app has
exactly one side panel (Instances) and a single `1` focus key; there's no
machinery for a second side panel or for moving focus between panels. Every
panel item below is gated on this, which is the reason to do it before the
smaller features rather than after.

lazydocker's model to copy: side panels each own a window, number keys
`1`-`6` jump to a panel, and the generic `SideListPanel` machinery in
`pkg/gui/panels` (already ported here) handles list behavior once a panel
exists. The missing part is the arrangement/focus layer — `arrangement.go`,
`focus.go` and `window.go` here were ported for the single-panel case.

- [ ] Generalize the arrangement/focus layer to more than one side panel
- [ ] Focus keys per panel (`1`-`n`), replacing the hardcoded `1`
- [ ] Decide on tab-cycling between side panels as well as number keys

## Missing vs lazydocker

### Side panels

lazydocker has six side panels; lazyincus has one. All of these need
panel-switching (above) first.

- [ ] **Images panel** — `incus image list` + delete. Straightforward once
      there's somewhere to put it.
- [ ] **Volumes panel** — storage pools / volumes.
- [ ] **Networks panel** — networks.
- [ ] **An "about"/credits surface** — lazydocker's Project panel hosted its
      credits tab, so dropping that panel left lazyincus with nowhere to put
      one (`CreditsTitle` is ported but unused).

Not planned as lazydocker has them:

- **Services / Project panels** — these were docker-compose specific, and the
  port assumed Incus had nothing to map them onto. That assumption is now
  wrong: see [incus-compose integration](#incus-compose-integration).

### Per-instance actions

- [ ] **Attach (`a`)** — the most substantive missing action. The Incus
      analog is `incus console <name>` (plus `--type vga` for VMs). Three
      translation strings were ported for it (`Attach`,
      `UnattachableInstanceError`, `DetachFromInstanceShortCut`) and none are
      wired to anything. `instanceExecShell` already shells out via
      `runSubprocessWithMessage`, so this is close to a copy of that handler.
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

- **Top tab** (per-instance process list) — also awkward on Incus: the API
  reports a process *count*, not a list.
- **Stats history / graphing** — the Stats tab is point-in-time only;
  lazydocker's `RecordedStats`/graph config machinery wasn't ported.
- **Non-English translations**, **Windows support**.
- **Event stream** — lazydocker consumes Docker's `/events`; lazyincus polls
  every 2s instead. Fine in practice; noted only for completeness.

### Cleanup

- [ ] **Unused translation strings** — thirteen are defined but never
      referenced: `MainTitle`, `GlobalTitle`, `ErrorOccurred`,
      `ConnectionFailed`, `UnattachableInstanceError`, `ForceRemove`,
      `Attach`, `NoInstance`, `RemoveWithForce`, `FilterList`,
      `SortInstancesByState`, `CreditsTitle`, `CannotDisplayEnvVariables`.
      Some mark genuinely half-ported features (attach); the rest are dead
      weight. Wire up or delete.

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

### 1. Project awareness (needed first, useful on its own)

**incus-compose creates one Incus project per compose project** — run
`incus-compose -p myapp up` and you get an Incus project `myapp` holding that
stack's instances, networks and volumes, plus a separate
`incus-compose-cache` project for pulled images.

lazyincus connects with `cliCfg.GetInstanceServer(cliCfg.DefaultRemote)` and
never touches projects. `GetInstanceServer` does honor whatever project the
user's remote is configured for (`remote.Project`, applied via `UseProject`
inside `shared/cliconfig/remote.go`), but nothing else — so **every
incus-compose stack is currently invisible in lazyincus** unless the user has
switched their `incus` CLI remote to that project. That's a real usability
hole today, independent of any compose features, since hand-made Incus
projects have exactly the same problem.

The client API needed is small: `GetProjectNames()` / `GetProjects()` to
enumerate, and either `client.UseProject(name)` or `cliCfg.ProjectOverride`
before `GetInstanceServer` to switch.

- [x] Show which project the instance list is scoped to (footer shows it
      next to the remote, e.g. `Incus v6.11 (colima/default) ●`)
- [x] A project switcher (`P`: menu of `GetProjectNames()`, re-scoping the
      client via `UseProject`)
- [ ] Decide whether an "all projects" aggregate view is worth it, or whether
      switching is enough

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

- [ ] **Service column / grouping** — read
      `user.label.incus-compose.service` and show it, or group the instance
      list by it. Cheap, and it's most of what lazydocker's Services panel
      actually gave you.
- [ ] **Health column** — `user.healthcheck.enabled` plus the healthd state
      would give lazyincus a health indicator, which the port dropped along
      with Docker's healthcheck support. Needs a look at where `ic-healthd`
      writes results before this is more than a guess.
- [ ] **`incus-compose` shell-outs** — `up`/`down`/`restart` on the selected
      project, in the `instanceExecShell` subprocess style. Optional, and it
      adds a second CLI dependency beyond `incus` itself.

### Caveats

- None of this is verified against a live incus-compose setup — the key
  names above were read out of `project/instance.go` on GitHub, not observed
  on a running instance. Confirm before building on them.
- Those keys are internal to incus-compose and carry no compatibility
  promise. If lazyincus depends on them, pin the commit they were read from
  the way the lazydocker port pin works, so a drift has somewhere to be
  checked against.

## Interactive Snapshots panel

The current Snapshots tab (main panel, alongside Stats/Logs/Config) is
read-only: a static table of name/taken-at/expires-at/stateful, matching
`incus info`'s own Snapshots table.

Adding create/restore/delete actions doesn't fit that tab model — those need
per-row selection and keybindings, which the plain-text main-panel tabs don't
support. The right shape is a second **side panel** (the way lazydocker
treats Images/Volumes/Networks as their own panels, not tabs), scoped to the
snapshots of the currently-selected instance. Gated on panel-switching.

- [ ] Side panel showing snapshots of the selected instance
- [ ] Create (prompt for name)
- [ ] Restore
- [ ] Delete, with confirmation matching the Instances panel's pattern

## Housekeeping

- [x] **Backfill the GitHub release for v0.2.0** — cut from its changelog
      section, and flagged not-latest so v0.3.0 keeps that badge. v0.1.0 is
      still tag-only; backfill it the same way if the releases page should
      be complete.
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
