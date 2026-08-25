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

Not planned:

- **Services / Project panels** — docker-compose only, no Incus analog.

### Per-instance actions

- [ ] **Attach (`a`)** — the most substantive missing action. The Incus
      analog is `incus console <name>` (plus `--type vga` for VMs). Three
      translation strings were ported for it (`Attach`,
      `UnattachableInstanceError`, `DetachFromInstanceShortCut`) and none are
      wired to anything. `instanceExecShell` already shells out via
      `runSubprocessWithMessage`, so this is close to a copy of that handler.
- [ ] **Open in browser (`w`)** — lazydocker opens the container's first HTTP
      port. Incus has no port-mapping concept, but "open `http://<ipv4>`" is
      the obvious translation, and `OSCommand.OpenLink` already exists unused.

Not planned:

- **Custom commands (`c`) / bulk commands (`b`)** — dropped by design, along
  with their config sections.

### Global

- [ ] **Edit/open config (`e` / `o`)** — lazydocker binds these on its
      Project panel. `OpenConfig`/`EditConfig` strings are ported and
      `OSCommand.OpenFile`/`EditFile` both exist, but no keybinding calls
      them. Cheapest real gap on this list.
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

- [ ] **Unused translation strings** — sixteen are defined but never
      referenced: `MainTitle`, `GlobalTitle`, `OpenConfig`, `EditConfig`,
      `ErrorOccurred`, `ConnectionFailed`, `UnattachableInstanceError`,
      `CannotKillChildError`, `ForceRemove`, `Attach`, `NoInstance`,
      `RemoveWithForce`, `FilterList`, `SortInstancesByState`,
      `CreditsTitle`, `CannotDisplayEnvVariables`. Some mark genuinely
      half-ported features (attach, open/edit config); the rest are dead
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
- [ ] **Profiles / projects / remotes** — no panels or switching UI for any
      of them. The daemon connection uses whichever remote is
      `default-remote` in the user's Incus config and never offers to change
      it.

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

- [ ] **GitHub release for v0.2.0** — the tag is pushed but no release was
      cut, so the releases page has no entry and no built binaries. The
      changelog section is ready to paste if that's wanted; worth deciding
      once rather than per-tag.

## Blocked

Needs an M3+ Apple Silicon machine or a native Linux host — nested
virtualization isn't available on the M1 Pro this was developed on. See open
questions 2 and 3 in
[CLAUDE.md](CLAUDE.md#open-questions--unverified-assumptions).

- [ ] Verify freeze/unfreeze against a real VM
- [ ] Verify exec into a real VM
