# Backlog

Ideas and follow-up work not yet scheduled. Not a roadmap or a commitment —
just a place to park things so they don't get lost. See
[CHANGELOG.md](CHANGELOG.md) for what's actually shipped and
[CLAUDE.md](CLAUDE.md) for why the port is scoped the way it is.

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
the git history. Adding a panel is one entry in that list plus its own
files; number keys, `tab`/`shift+tab` cycling, and
`gui.expandFocusedSidePanel` for when an even split is too cramped all
derive from it automatically.

## Missing vs lazydocker

### Side panels

Both have six, and both only when a compose file is local: Services is the
Incus analog of lazydocker's docker-compose-specific Services/Project
panels, and like lazydocker's it pushes the plain instance list down to
"Standalone Instances". See
[incus-compose integration](#incus-compose-integration).

- [ ] **An "about"/credits surface** — lazydocker's Project panel hosted its
      credits tab (`CreditsTitle` is ported but unused). Ours has no
      always-present panel to host it: Services is absent without a compose
      file, so this needs somewhere else.

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

- **Stats history / graphing** — the Info tab's stats are point-in-time only;
  lazydocker's `RecordedStats`/graph config machinery wasn't ported.
- **Non-English translations**, **Windows support**.
- **Event stream** — lazydocker consumes Docker's `/events`; lazyincus polls
  every 2s instead. Fine in practice; noted only for completeness.

### Cleanup

- [ ] **Unused translation strings** — nine are defined but never
      referenced: `MainTitle`, `GlobalTitle`, `ForceRemove`, `NoInstance`,
      `RemoveWithForce`, `FilterList`, `SortInstancesByState`,
      `CreditsTitle`, `CannotDisplayEnvVariables`. Dead weight now that
      attach is wired up. Wire up or delete.

## Not lazydocker-shaped

The interesting gaps aren't all inherited from lazydocker. Profiles,
projects, and remote-switching are Incus concepts with no Docker analog, so
they never show up in the comparison above and are worth considering on their
own merits.

- [ ] **Copy menu** — generalize `y` (currently hardcoded to first IPv4) into
      a menu: name / IPv4 / IPv6 / all addresses. The `Instance.Addresses`
      and `OSCommand.CopyToClipboard` plumbing is already generic; this is
      mostly a menu panel plus entries.
- [ ] **Horizontal truncation indicator** — a row wider than its panel is
      clipped silently at the view's edge, so a cut-off IPv4 column looks
      like a short address rather than a hidden one. gocui has nothing for
      this, and the right border column is already spoken for: `drawFrame`
      puts the vertical scrollbar thumb there. The fix that doesn't fight it
      is to clip the rows ourselves in `RerenderList`
      (`pkg/gui/panels/side_list_panel.go`), running each line of the
      rendered table through `utils.Truncate` at `View.InnerWidth()` — the
      same `…` the per-column widths in `pkg/gui/presentation` already use,
      just applied to the row. Watch the colour codes: `Truncate` measures
      display width with `runewidth`, which counts escape sequences, so it
      needs the `Decolorise` treatment `getPadWidths` gives them — otherwise
      it cuts rows that only look long, and can land the cut inside an
      escape sequence. lazydocker clips silently too, so there's no upstream
      behaviour to match here.
- [ ] **Remote switcher** — an `R` menu picking the remote the panels talk to,
      mirroring `P` for projects. `pkg/gui/remotes.go` alongside
      `projects.go`: the menu lists `cliconfig.Config.Remotes`, marked with
      `marker()`, and the reload afterwards is exactly
      `reloadAfterProjectChange` (which wants a scope-neutral name), since
      clearing every panel is also what drops the per-item clients pointing
      at the previous daemon. `NewIncusCommand` has to keep `cliCfg` rather
      than dropping it after connecting, and a `UseRemote` swaps `client` and
      `RemoteName`, re-runs `GetServer()` for the footer, and resets to
      all-projects — the new server's project list has nothing to do with the
      old one's. Three parts that aren't just copying the project switcher:
      the list needs filtering, since `GetInstanceServer` rejects anything
      with `Public` set or `Protocol != "incus"` (`cliconfig/remote.go`), so
      `images:` and OCI remotes would be entries that only ever error;
      connecting can hang or fail where switching project can't, so it wants
      `WithWaitingStatus` off the main goroutine and must keep the existing
      client on failure rather than leaving the app with none; and it should
      stay session-only, leaving `incus remote switch` as the persistent
      path. The shell-outs are already handled: `--remote` set `INCUS_REMOTE`
      for the process, so a switcher only has to keep that variable current
      as it swaps the client. Verifying the failure paths needs a second
      reachable daemon, so the unreachable-remote case is the part likeliest
      to ship untested.
- [ ] **Profiles** — no panel. (Projects have a switcher — see
      [incus-compose integration](#incus-compose-integration); remotes are
      above.)
- [x] **`--remote` flag** — names the remote for the session, applied as
      `INCUS_REMOTE` so the shell-outs follow the panels.

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
7.2+ — of the *daemon*, not the client library this port builds against, so
v7.3.0 on our side says nothing about it and an older server refuses every
verb. Commands mirror compose: `up`,
`down`, `start`, `stop`, `restart`, `list`/`ps`, `logs`, `exec`, `config`,
`build`.

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
| `user.healthcheck.enabled` / `user.healthcheck.*` | healthcheck opt-in and its test/interval/retries, driven by an `ic-healthd` sidecar; `user.healthcheck.status` is where that sidecar writes its verdict |
| `user.incus-compose.managed` | marks every instance and project compose owns |
| `user.image_alias` | the image reference the compose file named |
| `user.incus-compose.oneoff` | marks a one-off (`run`-style) instance |
| `environment.<KEY>` | the service's environment variables |

Instances are named `<service>-<index>` (`web-1`, `app-1`).

Two things fall out of this. Those keys live in `ExpandedConfig`, which
`RefreshInstanceDetails` already fetches — so both are presentation work on
data lazyincus has in hand, not new API calls.

- [x] **Service column** — opt-in `service` column reading
      `user.label.incus-compose.service`.
- [x] **Health column** — opt-in `health` column. `ic-healthd` writes its
      verdict straight onto the instance as `user.healthcheck.status`, which
      is the only key it writes; the opt-in it consults
      (`user.healthcheck.enabled`) can sit on the Incus project instead, and
      `ExpandedConfig` expands profiles rather than projects, so status's
      presence is the signal and the opt-in is no use for this.
- [ ] **Health in the status column** — lazydocker renders health as a
      substatus beside the container's state, styled by
      `containerStatusHealthStyle` (`long`/`short`/`icon`), where ours is a
      column of its own. The inline form spends less width on a panel that
      rarely has any to spare.
- [x] **Image column** — opt-in `image` column reading `user.image_alias`,
      the reference the compose file named. Incus's own
      `volatile.base_image` is a fingerprint, so this is the only place an
      instance's image appears by name.
- [x] **A services panel** — the other half of what lazydocker's Services
      panel gave you, shipped: when a compose file is in the working
      directory, one row per service it declares, and the instances panel
      becomes "Standalone Instances" without them. See
      [CLAUDE.md](CLAUDE.md#services).

      The panel reads the compose file rather than the daemon, which is what
      buys the thing no column could: a service that's down still gets a
      row, in state `none`. Grouping a stack inside the flat instances panel
      is what this makes unnecessary — headers there would have meant
      `SideListPanel[*commands.Instance]` becoming a panel over a
      header-or-instance row, with every keybinding, `OnSelect`/`OnClick`
      and the snapshots panel having to no-op on a header, for a separator
      on a list the name sort already orders.

### 3. Project panel

lazydocker's sixth side panel. Its list, its local-project gate
(`CannotManageNonLocalService`) and its compose verbs all live in the
Services panel now — a list of every compose project on the server was rows
nothing could act on, so it went with the rewrite. What's left is the part
of lazydocker's panel that was a main-panel tab rather than the list:

- [x] **The verbs** and **the compose config tab** — shipped on the
      Services panel, per-service.
- [ ] **Credits tab** — the home the
      [missing credits surface](#side-panels) is waiting for. It lost the
      panel it was going to live on, so it needs somewhere else.
- [ ] **Aggregate logs tab** — lazydocker tails every container in the
      project at once. `incus-compose logs -f` is the analog and is in the
      Services panel's `C` menu as a subprocess; an in-panel tab still
      wants merged streams, and `TailConsoleLog` is per-instance and
      drain-on-read. The same gap leaves the Logs tab a hint rather than
      output on a service's own row, where a replicated service means
      several streams; a replica's own row shows that replica's log.

### Caveats

- The CLI surface above was read off `incus-compose 1.3.4` on macOS, where
  `--remote` takes `$INCUS_REMOTE`, `-p` is the project name and `-P` the
  project directory. The daemon floor above costs only the verbs:
  `incus-compose config` resolves the compose file without touching the
  daemon, so the panel's rows, tabs and hidden-ness are unaffected by it and
  were confirmed on 6.11.
- Those keys are internal to incus-compose and carry no compatibility
  promise, so they're pinned the way the lazydocker port is: everything
  above was read from
  [`f350005`](https://github.com/lxc/incus-compose/commit/f35000561ac2065435a45116e169b4cbe7612173)
  (2026-09-17), the health constants from `shared/health.go`. Re-check
  against that commit if a column ever goes blank.
- The keys were also confirmed on a live stack at that point, which the
  service column never had been — it was tested by setting the label by
  hand. Two keys the table above missed turned up there:
  `user.incus-compose.managed` on every instance and project compose owns,
  and `user.image_alias`, now the image column.
- `U` (pull and recreate) fails incus-compose's own way on a stack with a
  bind mount or device passthrough (`error="failed to add a bind-mount for
  service <name>: not on the same host"`) whenever the daemon isn't local —
  a colima VM included. `--recreate` re-validates those sources, and
  incus-compose refuses when it isn't running on the same host as the
  daemon. Not a lazyincus bug and nothing to fix here; plain `u` doesn't
  re-create an existing instance, so it doesn't hit this.
- A `--recreate` that fails for any reason is rolled back by incus-compose
  deleting the resources it just created ("Deleting resources
  project=<name>"), so `U` reads as having deleted the stack. The error
  left on screen is the rollback's own - observed as `delete network
  default [error: The network is currently in use]`, the project's bridge
  still carrying an instance the compose file no longer declares - and it
  hides whatever actually failed. `--debug` outside lazyincus is the way
  to see that one.

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

Shipped, but never seen working end to end, for want of the one thing the
development machine can't provide: a real VM instance. That needs a Linux
machine running Incus directly, or — when the daemon itself runs inside a
VM, as it does here — hardware nested virtualization, which on Apple
Silicon means an M3 or later with macOS 15+. Without `/dev/kvm` inside the
guest, `incus launch ... --vm` fails with `KVM support is missing (no
/dev/kvm)`, and no hypervisor or Incus flag substitutes for it.

- [ ] Verify freeze/unfreeze against a real VM. Both are documented as
      container-oriented actions, and `p` doesn't check `IsVM()` before
      offering them — error, silent no-op or actual suspend is unconfirmed.
- [ ] Verify exec into a real VM. `incus exec` needs the Incus guest agent
      running inside the VM; without it the shell-out fails with whatever
      the CLI prints. No special-casing was added.
