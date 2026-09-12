# User Config

## Opening the user config

`o` opens `config.yml` with the configured open command (`oS.openCommand`),
and `O` opens it in `$VISUAL`/`$EDITOR`. Both are global keybindings — they
work from any panel. You can equally well just open the file yourself.

lazyincus creates the file automatically on first run if it doesn't exist
yet.

### Reloading

Changes take effect without restarting lazyincus: an edit made with `O` as
soon as the editor exits, any other edit within about two seconds. Two
options are the exception and still need a restart:

- `gui.screenMode` — it only seeds the initial screen mode, which `+`/`_`
  then own.
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
| `language` | string | `"en"` | Only English is supported by this MVP; the field exists for forward-compatibility with lazydocker's i18n scaffolding. |
| `scrollPastBottom` | bool | `false` | Whether you can scroll the main panel past its last line. |
| `mouseEvents` | bool | `false` | Set `true` to **disable** mouse interaction (the YAML key is `mouseEvents` even though it toggles ignoring them — inherited as-is from lazydocker). |
| `theme.activeBorderColor` | []string | `[green, bold]` | Border color/attributes for the focused panel. |
| `theme.inactiveBorderColor` | []string | `[default]` | Border color/attributes for unfocused panels. |
| `theme.selectedLineBgColor` | []string | `[blue]` | Background color of the selected list row. |
| `theme.optionsTextColor` | []string | `[blue]` | Color of the keybinding hints in the bottom line. |
| `returnImmediately` | bool | `false` | Skip the "press enter to return to lazyincus" prompt after a subprocess (e.g. `incus exec`) finishes. |
| `wrapMainPanel` | bool | `true` | Word-wrap the main panel's content. |
| `expandFocusedSidePanel` | bool | `false` | Give the focused side panel the space the others aren't using, collapsing them to their title and first row. Falls back to an even split when the terminal is too short to fit them all collapsed. |
| `sidePanelWidth` | float | `0.3333` | Fraction of screen width used by the Instances side panel. |
| `showBottomLine` | bool | `true` | Show the bottom status/keybinding line. |
| `screenMode` | string | `"normal"` | Initial screen mode: `normal`, `half`, or `full` (`fullscreen` is accepted as an alias). |
| `instanceStatusStyle` | string | `"long"` | Instance status display: `long` (full words), `short` (one/two characters), or `icon`. |
| `border` | string | `"rounded"` | Panel border style: `rounded`, `single`, `double`, or `hidden`. |
| `instanceColumns` | []string | `[name, status, type, ipv4, ipv6, snapshots]` | Which columns the Instances panel shows, and in what order. Valid values: `name`, `status`, `type`, `ipv4`, `ipv6`, `service`, `snapshots`. Unknown values are ignored; list any subset to hide the rest. `service` is off by default: it shows the [incus-compose](https://github.com/lxc/incus-compose) service an instance came from, and is blank for instances created any other way. |

### Top level

| Key | Type | Default | Meaning |
|---|---|---|---|
| `confirmOnQuit` | bool | `false` | Prompt for confirmation when quitting with `q`/`esc` and no other panel is open. |
| `oS.openCommand` | string | macOS: `open {{filename}}`; Linux: `sh -c "xdg-open {{filename}} >/dev/null"` | Command used to open a file. |
| `oS.openLinkCommand` | string | macOS: `open {{link}}`; Linux: `sh -c "xdg-open {{link}} >/dev/null"` | Command used to open a URL. |
| `oS.copyToClipboardCommand` | string | auto-detected | Command that copied text (e.g. an instance's IPv4 address, via `y`) is piped into on stdin. When unset, the first of `pbcopy`, `wl-copy`, `xclip -selection clipboard -in`, `xsel --clipboard --input` found on `PATH` is used. |
| `ignore` | []string | `[]` | List rows are hidden when any displayed column contains one of these substrings — status and IP addresses included, not just the name. |

## What's not here (yet)

Ported from lazydocker's config in this MVP: `gui`, `confirmOnQuit`, `oS`,
`ignore`. Deliberately **not** ported (see
[BACKLOG.md](../BACKLOG.md#missing-vs-lazydocker) for
why): `commandTemplates` (docker-compose command templates), `customCommands`,
`bulkCommands`, `stats`/`graphs`, `replacements`, and `logs` (`since`/`tail`/
`timestamps` don't map onto Incus's console-log snapshot model).
