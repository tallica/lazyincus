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

lazydocker has six with a compose file local; lazyincus has three, or five
with incus-compose installed, having folded images, volumes, networks
and profiles into one tabbed Resources panel. Stacks and Services are the Incus
analog of lazydocker's docker-compose-specific Project/Services panels, and
like lazydocker's they push the plain instance list down to "Standalone
Instances". See [incus-compose integration](#incus-compose-integration).

- [ ] **An "about"/credits surface** — lazydocker's Project panel hosted its
      credits tab (`CreditsTitle` is ported but unused). Ours has no
      always-present panel to host it: Stacks is absent without
      incus-compose, so this needs somewhere else.

### Per-instance actions

- [x] **Attach (`a`)** — `incus console <name>`.
- [ ] **Graphical console (`A`)** — a VM's VGA output, through
      `incus console <vm> --type vga`, which mirrors the SPICE socket and
      starts a viewer: the CLI config's `defaults.console_spice_command`,
      else `remote-viewer`, else `spicy`. Started in the background, not as
      a subprocess: the viewer is a window of its own, and the command ends
      when it closes. With no viewer the CLI prints the socket path and
      waits, unseen in the background, so refuse up front when none is on
      `PATH` and the config names none - and on a container or a stopped
      VM. On macOS that means installing virt-viewer.
- [ ] **Open in browser (`w`)** — lazydocker opens the container's first HTTP
      port. Incus's port mapping is a proxy device; for an instance without
      one, "open `http://<ipv4>`" is the obvious translation, and
      `OSCommand.OpenLink` already exists. The same doubt that kept a
      stack's ports from being links applies: nothing says a port speaks
      HTTP. See [Clickable links](#what-gocui-master-makes-possible).

Not planned:

- **Custom commands (`c`) / bulk commands (`b`)** — lazydocker's shape,
  dropped by design along with its config sections: named entries in the
  config, templated over the selected item, opened as a menu. What replaces
  it is a typed passthrough rather than a curated list — see
  [Typed commands](#typed-commands).

### Global

- [x] **Edit/open config (`e` / `o`)** — bound globally as `o` (open) and
      `O` (edit).
- [ ] **Cheatsheet generator** — lazydocker generates `docs/keybindings/*.md`
      from its i18n set via `scripts/cheatsheet`. Here the README keybinding
      table is hand-maintained, which is why CLAUDE.md has to carry a
      reminder to keep it current.
- [x] **Event stream** — lazyincus listens to Incus's lifecycle and
      operation events; see [docs/Incus.md](docs/Incus.md).
- [ ] **More row statuses from operations** — `operationStatuses` covers
      starting, stopping, restarting, restoring, freezing and unfreezing.
      "Deleting instance", "Rebuilding instance" and "Migrating instance"
      can take a while too and could read `deleting`, `rebuilding`,
      `migrating` the same way, each needing its entries in
      `DisplayStatus`'s maps and `StatusColor`. A deleted instance's mark
      has no row left to end on, so check it comes off with the listing
      that drops the row.
- [ ] **`UseProject` against a connecting stream** — the warnings got a
      client made with the connection because `UseProject` copies the
      client's `skipEvents` unguarded while an event stream connecting on
      it writes it ([docs/Incus.md](docs/Incus.md), "Warnings"). `clientFor`
      and the per-instance clients a listing makes do the same; the race
      detector hasn't caught them, but the window is the same one.
- [ ] **Events for a restricted certificate** — `ListenForEvents` asks for
      every project's events whenever the panels list every project. A
      certificate restricted to some projects may be refused that, and
      `watchEvents` then retries with backoff for as long as the app runs,
      logging each refusal; the polls carry on, so nothing breaks, but the
      stream never opens. Fall back to the client's own project's events,
      or one listener per allowed project, and stop retrying a refusal
      that won't change. Untried: the development certificate isn't
      restricted, so `incustest`'s `Listen` would need to refuse
      all-projects for a test to see it.
- [ ] **Poll less now that events carry the changes** — what's polled, and
      why, is the table in [docs/Panels.md](docs/Panels.md#what-keeps-them-current).
      The 2s poll is the big one: every instance's config, state and
      snapshots, in every project, stream or not, the services' listing
      with it. Most of that the events already bring; what it's there for
      is CPU, memory, processes, disk and addresses. Ways to shrink it:
      - Tick the Info tab off the selected instance's own
        `GetInstanceState`, and slow the full listing right down while
        the stream is open.
      - The IPv4 column then needs an address arriving after a start:
        list a few times after an `instance-started` - at 1s, 3s, 10s -
        rather than every 2s for good.
      - Judge the connection with `GET /1.0` rather than a full listing;
        a dropped stream already hurries it.
      - Poll the Resources panel's tab on screen, not all four: a volume's
        usage is the main thing left to poll there.
      - Refresh the instances when an "Updating snapshot" operation ends:
        an instance snapshot's edit sends no lifecycle event, so it waits
        for the poll ([docs/Incus.md](docs/Incus.md), "Events").
      - Add `instance-ready` to `eventRefreshes`, for the `Ready` status.

      Measure first: a listing's cost on a daemon with a few hundred
      instances is untried, the development daemons having a dozen or so.

Not planned:

- **Stats history / graphing** — the Info tab's stats are point-in-time only;
  lazydocker's `RecordedStats`/graph config machinery wasn't ported.
- **Non-English translations**, **Windows support**.

### Cleanup

- [ ] **Unused translation strings** — nine are defined but never
      referenced: `GlobalTitle`, `ErrorOccurred`, `ForceRemove`,
      `NoInstance`, `RemoveWithForce`, `FilterList`, `SortInstancesByState`,
      `CreditsTitle`, `CannotDisplayEnvVariables`. Dead weight now that
      attach is wired up. Wire up or delete.

## Not lazydocker-shaped

The interesting gaps aren't all inherited from lazydocker. Profiles,
projects, and remote-switching are Incus concepts with no Docker analog, so
they never show up in the comparison above and are worth considering on their
own merits.

- [x] **Copy menu** — `y` is a menu of what the item has to copy, on
      every list. A lease's address isn't among them: the Leases tab has
      no cursor for a row to be chosen with.
- [x] **Horizontal truncation indicator** — `SideListPanel` renders its
      rows at the view's width, clipping each through a colour-aware
      `utils.Truncate`, and re-renders on a width change. lazydocker clips silently, so there
      was no upstream behaviour to match.
- [x] **Remote switcher** — `R`, a menu of the CLI's instance remotes;
      the session's command adopts the new connection in place
      (`UseRemote`), so whatever holds it follows. Session-only, leaving
      `incus remote switch` as the persistent path.
- [x] **Profiles** — the Resources panel's fourth tab, with the other
      resources' `u`, `c` and `d`. (Projects have a switcher — see
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

### What gocui master makes possible

Available since the move off lazydocker's 2024 pin; none of it wired up.

- [ ] **Clickable links** - `View.AutoRenderHyperLinks` on the main view
      turns URLs into terminal hyperlinks. Not for a stack's published
      ports: a port says nothing of its protocol, so an `http://` link
      would be a guess for anything that isn't a web server. `y` on the
      stack copies a port's address instead.
- [ ] **Double-click** - `ViewMouseBindingOpts.IsDoubleClick`. A double
      click on a row could do what enter does, focusing the main panel.
- [ ] **Recentre on scroll** - `View.FocusPoint` keeps the selection in the
      middle once it leaves the view, the way lazygit scrolls; lazyincus
      moves a line at a time, as lazydocker does. A UX choice to make, not
      a gap.

### Typed commands

`incus-compose` has this already, as an extension rather than a compose
verb: `incus-compose incus <args>` runs the real incus CLI in the compose
project's context — `incus-compose incus list` in a stack's directory lists
that project's instances, not the default project's. The value isn't a
command list, it's the scoping: the wrapper injects the context so you
don't type it. lazyincus knows the same context from whichever row the
cursor is on, which is the argument for a typed passthrough over
lazydocker's configured menu.

Both entries below are one prompt and one runner, so the first should write
`runTypedCommand(binary, prefill)` and the second should be a caller and a
gate. Deliberately out of scope for a first version: command history,
completion, and a menu of saved commands — each is state of its own, and
none of it is needed to make typing a command useful.

- [ ] **Typed incus command (`:`)** — an editable prompt, submitted to
      `incus` as a subprocess. The prompt seeds with the selected row's
      `--project` (`projectCLIArgs`), cursor after it, and runs on the
      row's remote the way `instanceCmd` does, so the common case
      is typing the verb alone and the uncommon one is deleting a prefix.
      Reuses `openTextPrompt`, the one Stacks' `a` asks for a directory
      with, and `runSubprocess`, so an interactive command works
      and the panels refresh afterwards the way `composeRun` refreshes
      them. Tokenize respecting quotes rather than splitting on whitespace,
      and don't route through `sh -c`: pipes and `$(...)` aren't worth an
      unconfirmed prompt that runs what it's given.
- [ ] **Typed incus-compose command (`;`)** — the same prompt against
      `incus-compose`, on the Stacks and Services panels, run through
      `ComposeCmd` in the selected stack's directory and on its remote, as
      `composeRun` does. Nothing to prefill
      beyond the selected service's name as a trailing argument.

Keys aren't settled: `:` and `;` are both free and read as a pair on one
physical key, `!` being the alternative if `;` is too easy to hit by
accident.

## incus-compose integration

[incus-compose](https://github.com/lxc/incus-compose) is a drop-in
replacement for `docker compose` that runs an unmodified `compose.yaml`
against Incus, pulling OCI images straight from docker.io/ghcr.io via Incus's
native OCI support. It started as [bketelsen/incus-compose] and now lives
under the LXC org; its docs call it stable, and it needs Incus 7.0.1 LTS or
7.2+ — of the *daemon*, not the client library this port builds against, so
our client's version says nothing about it and an older server refuses
every verb. Commands mirror compose: `up`,
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

Two things fall out of this. Those keys live in `ExpandedConfig`, which the
instance listing already carries — so both are presentation work on data
lazyincus has in hand, not new API calls.

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
      panel gave you, shipped: for the stack selected in Stacks, one row
      per service its compose file declares, and the instances panel
      becomes "Standalone Instances" without them. See
      [docs/Panels.md](docs/Panels.md#services).

      The panel reads the compose file rather than the daemon, which is what
      buys the thing no column could: a service that's down still gets a
      row, in state `none`. Grouping a stack inside the flat instances panel
      is what this makes unnecessary — headers there would have meant
      `SideListPanel[*commands.Instance]` becoming a panel over a
      header-or-instance row, with every keybinding, `OnSelect`/`OnClick`
      and the snapshots panel having to no-op on a header, for a separator
      on a list the name sort already orders.

### 3. Project panel

lazydocker's sixth side panel, shipped as Stacks
([docs/Panels.md](docs/Panels.md#stacks)): a list of compose project
directories, the local one and any added with `a`, each on the Incus
remote it was added for, whose selection the Services panel follows. It
lists directories rather than every compose project on the server, a
project with no compose file in reach being rows nothing could act on.
lazydocker's local-project gate (`CannotManageNonLocalService`) is what
that choice replaces. What's left is the part of lazydocker's panel that
was a main-panel tab rather than the list:

- [x] **The verbs** and **the compose config tab** — shipped on both
      panels: per service on Services, per stack on Stacks.
- [ ] **Credits tab** — the home the
      [missing credits surface](#side-panels) is waiting for. It lost the
      panel it was going to live on, so it needs somewhere else.
- [ ] **Interleaved logs tab** — lazydocker tails every container in the
      project at once, interleaved. `incus-compose logs -f` is the analog,
      on `M` as a subprocess. The stack's Logs tab stacks every instance's
      `TailConsoleLog` buffer under a heading; what's left is ordering the
      streams against each other in a tab, which those buffers carry
      nothing for.

The stack's Info tab, beyond what shipped:

- [ ] **Declared but not created** — Usage lists the volumes and networks
      the instances' devices use, so one the compose file declares that the
      stack doesn't have yet isn't there. Showing it as `(not created)`
      needs `composeConfigOutput` to read the top-level `volumes:` and
      `networks:`, matched against the project's.
- [ ] **Usage on a stopped stack** — with nothing running it's `N/A` and
      zeros throughout. It could wait until something runs, at the cost of
      the disks and networks it also lists.
- [ ] **Instance Info's column** — its `Disk:` and `Network:` entries keep
      a padding of their own, so a long volume name puts their values out
      of line with the counters above; the stack's Usage section moves
      the whole column over instead.

### 4. Backups (shipped)

`incus-compose backup` snapshots a project's data volumes into a
`<project>-backup` Incus project. The Backups tab beside Snapshots
lists the selected stack's, and the Volumes panel marks the volumes that
hold them; both are in [docs/Panels.md](docs/Panels.md#backups).

- [x] **Mark backup volumes in the Volumes panel** — a marker in the users
      column rather than a filter: a filter would hide what the panel
      exists to show, a pool's space being spent.
- [x] **A Backups panel** — a tab in the Snapshots slot rather than a
      panel of its own, backups being to a stack what snapshots are to an
      instance. `n`, `d`, `D` (`--keep-last`), `r` (the stack or one
      service) and `v`.
- [ ] **Back up one service** — `create` takes `SERVICE...`; `n` backs up
      the whole stack. Worth adding if restoring one service turns out to
      be the common case.
- [ ] **Partial and missing restore points** — `verify`'s other statuses
      (`backup volume missing`, `restore point missing`, `no longer in the
      project`, `not in this backup`) were read out of `backup_verify.go`
      at [`13b1b7a`](https://github.com/lxc/incus-compose/commit/13b1b7a445dc0fe173ad329d4999ca8c31252fb8),
      not seen. Every live run so far was a stack with two volumes, every
      volume `ok`.

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
- [x] **Attach built binaries to releases** — GoReleaser builds macOS and
      Linux, amd64 and arm64, on every `v*` tag. See
      [CLAUDE.md](CLAUDE.md#releasing).

## Blocked

Nothing now. Freeze/unfreeze and exec shipped before there was a real VM
to try them on; both have since been verified against one.

- [x] Verify freeze/unfreeze against a real VM. `p` freezes a running VM,
      which then reads `frozen`, and resumes it, with no `IsVM()` check
      needed.
- [x] Verify exec into a real VM. It goes through the Incus guest agent; a
      VM without one still gets whatever the CLI prints, which hasn't been
      seen yet.
