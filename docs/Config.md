# User Config

## Opening the user config

`o` opens `config.yml` with the configured open command (`oS.openCommand`),
and `O` opens it in `$VISUAL`/`$EDITOR`. Both are global keybindings — they
work from any panel. You can equally well just open the file yourself.

lazyincus creates the file automatically on first run if it doesn't exist
yet.

### Reloading

Changes take effect without restarting lazyincus: an edit made with `O` as
soon as the editor exits, any other edit within about two seconds. Four
options are the exception and still need a restart:

- `gui.screenMode` — it only seeds the initial screen mode, which `+`/`_`
  then own.
- `gui.expandFocusedSidePanel` — likewise, it only seeds the startup state;
  `=` owns it from then on.
- `gui.showAllSnapshots` — the same, with `e` on the Snapshots panel.
- `gui.language` — English is the only supported language anyway.

A YAML error leaves the config in effect alone. An edit made with `O`
reports it in a panel; the file watcher logs it quietly (it may have caught
a half-written save) and retries on the next change.

See [config.example.yml](../config.example.yml) for a fully commented copy
of every default value, ready to copy from.

### Locations

- Linux: `~/.config/lazyincus/config.yml`
- macOS: `~/.config/lazyincus/config.yml` if that directory already exists,
  otherwise `~/Library/Application Support/lazyincus/config.yml`

(Windows isn't supported — see [BACKLOG.md](../BACKLOG.md#missing-vs-lazydocker).)

Checked in this order:
1. The `CONFIG_DIR` environment variable, which points directly at the
   directory to use.
2. The `XDG_CONFIG_HOME` environment variable, which is joined with
   `lazyincus` to form the directory.
3. `~/.config/lazyincus`, if it already exists - so macOS users who already
   have a config there (e.g. from dotfiles synced from a Linux machine)
   get picked up without moving anything.
4. The platform default: `~/.config/lazyincus` on Linux,
   `~/Library/Application Support/lazyincus` on macOS.

Only non-zero-value keys need to be set explicitly — omitted keys fall back
to the defaults in [config.example.yml](../config.example.yml) (the struct
tags use `omitempty`, so lazyincus never writes zero-value keys back to
your file either).

## Field reference

### `gui`

| Key | Type | Default | Meaning |
|---|---|---|---|
| `scrollHeight` | int | `2` | Lines scrolled at a time in the main panel. |
| `language` | string | `"en"` | Only English is supported; the field exists for forward-compatibility with lazydocker's i18n scaffolding. |
| `scrollPastBottom` | bool | `false` | Whether you can scroll the main panel past its last line. |
| `mouseEvents` | bool | `false` | Set `true` to **disable** mouse interaction (the YAML key is `mouseEvents` even though it toggles ignoring them — inherited as-is from lazydocker). |
| `theme.activeBorderColor` | []string | `[green, bold]` | Border color/attributes for the focused panel. |
| `theme.inactiveBorderColor` | []string | `[default]` | Border color/attributes for unfocused panels. |
| `theme.selectedLineBgColor` | []string | `[blue]` | Background color of the selected list row: a color name or a hex value. Where the palette's blue is too light to read the row's colored text on, a dark grey such as `["#515151"]` works on any dark theme. |
| `theme.optionsTextColor` | []string | `[blue]` | Color of the keybinding hints in the bottom line. |
| `returnImmediately` | bool | `false` | Skip the "press enter to return to lazyincus" prompt after a subprocess (e.g. `incus exec`) finishes. |
| `wrapMainPanel` | bool | `true` | Word-wrap the main panel's content. |
| `expandFocusedSidePanel` | bool | `false` | Start with the focused side panel given the space the others aren't using, collapsing them to their title and first row; `=` toggles it during a session. Falls back to an even split when the terminal is too short to fit them all collapsed. |
| `showAllSnapshots` | bool | `false` | Start with the Snapshots panel listing every instance's snapshots rather than the selected instance's; `e` on that panel toggles it during a session. |
| `collapseStacksElsewhere` | bool | `true` | On a remote with no stack listed, collapse Stacks and Services to their titles and start the focus on Instances; focusing either swaps them back. `false` keeps the even split and the focus on Stacks. |
| `sidePanelWidth` | float | `0.3333` | Fraction of screen width used by the side panels' column. |
| `showBottomLine` | bool | `true` | Show the bottom status/keybinding line. |
| `screenMode` | string | `"normal"` | Initial screen mode: `normal`, `half`, or `full` (`fullscreen` is accepted as an alias). |
| `instanceStatusStyle` | string | `"long"` | Instance status display: `long` (full words), `short` (one/two characters), or `icon`. Applies to the Services and Stacks panels' rolled-up statuses too, and a stack's `error`, `unreachable` and `connecting`. |
| `border` | string | `"rounded"` | Panel border style: `rounded`, `single`, `double`, or `hidden`. |
| `instanceColumns` | []string | `[name, status, health, type, ipv4, snapshots]` | Which columns the Instances panel shows, and in what order. Valid values: `name`, `status`, `type`, `ipv4`, `ipv6`, `project`, `service`, `health`, `image`, `snapshots`. `project` is added automatically whenever the list spans more than one project. Unknown values are ignored; list any subset to hide the rest. `service`, `health` and `image` read [incus-compose](https://github.com/lxc/incus-compose) config keys, so all three are blank for instances created any other way: the service an instance came from, its `ic-healthd` health (`starting`, `healthy`, `unhealthy`, `stopped`, or `unknown` for a service that declared no healthcheck), and the image reference the compose file named. |
| `serviceColumns` | []string | `[name, status, replicas, health, ipv4, snapshots]` | Which columns the Services panel shows, and in what order. Valid values: `name`, `status`, `replicas`, `type`, `ipv4`, `ipv6`, `health`, `image`, `snapshots` — the same renderers `instanceColumns` uses, over the service's instances rolled up, plus `replicas` (how many instances the service has against how many the compose file declared, blank when they agree — the figure before the slash is the number of replica rows underneath). The project is in the panel title rather than a column, every row sharing it, followed by the stack's remote when that isn't the session's. Only read when the Services panel is there at all. |

### Top level

| Key | Type | Default | Meaning |
|---|---|---|---|
| `confirmOnQuit` | bool | `false` | Prompt for confirmation when quitting with `q`/`esc` and no other panel is open. |
| `remotes.<name>.readOnly` | bool | `false` | Refuse every key that would change something on the remote the incus CLI calls `<name>`, saying why: starting, stopping, deleting, snapshots, editing config, the compose verbs, and the `E`/`a` shells, which can change anything inside. Browsing, logs, copying, filtering and switching remote or project still work, and so do the Stacks panel's own list edits, which change `state.yml` rather than the remote. A stack's keys follow its own remote, not the session's. The footer says `read-only` while the session's remote is. `--read-only` makes every remote read-only for the run. |
| `oS.openCommand` | string | macOS: `open {{filename}}`; Linux: `sh -c "xdg-open {{filename}} >/dev/null"` | Command used to open a file. |
| `oS.openLinkCommand` | string | macOS: `open {{link}}`; Linux: `sh -c "xdg-open {{link}} >/dev/null"` | Command used to open a URL. Nothing opens one yet; it's kept for [open in browser](../BACKLOG.md#per-instance-actions). |
| `oS.copyToClipboardCommand` | string | auto-detected | Command that copied text (e.g. an instance's IPv4 address, via `y`) is piped into on stdin. When unset, the first of `pbcopy`, `wl-copy`, `xclip -selection clipboard -in`, `xsel --clipboard --input` found on `PATH` is used. |
| `ignore` | []string | `[]` | List rows are hidden when any displayed column contains one of these substrings — status and IP addresses included, not just the name. |

## State

`state.yml`, beside `config.yml`, is what lazyincus remembers rather than
what you configure: the stacks added to the Stacks panel with `a`, as a
`stacks` list of absolute directories, each after the remote it was added
for (`pve01:/home/me/deployments/caddy`). One with no remote - from
before stacks had one, or typed in by hand - is pinned to the remote
lazyincus starts on, the next time it starts. lazyincus writes it, where
`config.yml` is only ever yours to change. It's safe to edit by hand, and
a missing file is an empty list.

## What's not here (yet)

Ported from lazydocker's config so far: `gui`, `confirmOnQuit`, `oS`,
`ignore`. Deliberately **not** ported (see
[BACKLOG.md](../BACKLOG.md#missing-vs-lazydocker) for
why): `commandTemplates` (docker-compose command templates), `customCommands`,
`bulkCommands`, `stats`/`graphs`, `replacements`, and `logs` (`since`/`tail`/
`timestamps` don't map onto Incus's console-log snapshot model).
