# Backlog

Ideas and follow-up work not yet scheduled. Not a roadmap or a commitment —
just a place to park things so they don't get lost. See
[CHANGELOG.md](CHANGELOG.md) for what's actually shipped and
[CLAUDE.md](CLAUDE.md) for why the current MVP is scoped the way it is.

## Interactive Snapshots panel

The current Snapshots tab (main panel, alongside Stats/Logs/Config) is
read-only: a static table of name/taken-at/expires-at/stateful, matching
`incus info`'s own Snapshots table.

Adding create/restore/delete actions doesn't fit that tab model — those
need per-row selection and keybindings, which the plain-text main-panel
tabs don't support. The right shape is a second **side panel** (the way
lazydocker treats Images/Volumes/Networks as their own panels, not tabs),
scoped to the snapshots of the currently-selected instance. That's a
bigger step than anything shipped so far: it needs panel-switching
infrastructure the app doesn't have yet (currently there's exactly one
side panel, Instances).

Rough shape once picked up:
- New side panel showing snapshots of the selected instance.
- Keybindings: create (prompt for name), restore, delete (with confirm,
  matching the existing Instances panel's delete-confirmation pattern).
- Some way to switch focus between the Instances panel and the Snapshots
  panel (lazydocker uses number keys / tab-cycling between side panels).

## Other known gaps (see CLAUDE.md / README "What's not here yet")

- Images, Networks, Volumes panels.
- Services/Project panels (lazydocker's docker-compose view — Incus has
  no direct equivalent to map this onto).
- Custom and bulk commands system.
- Top tab (per-instance process list) and historical resource-usage
  graphing (the current Stats tab is point-in-time only).
- Non-English translations.
- Windows support.
