# User Config

## Opening the user config

There is currently no in-app keybinding to open the config file (lazydocker's
`o`/`e` shortcuts lived in its Project panel, which lazyincus doesn't have in
this MVP — see [CLAUDE.md](../CLAUDE.md)). Open `config.yml` directly in your
editor of choice instead.

lazyincus creates the file automatically on first run if it doesn't exist
yet. Changes only take effect after restarting lazyincus.

See [config.yml.example](../config.yml.example) for a fully commented copy
of every default value, ready to copy from.

### Locations

- Linux: `~/.config/lazyincus/config.yml`
- macOS: `~/Library/Application Support/lazyincus/config.yml`

(Windows isn't supported by this MVP — see CLAUDE.md.)

The location can be overridden two ways, checked in this order:
1. The `CONFIG_DIR` environment variable, which points directly at the
   directory to use.
2. The `XDG_CONFIG_HOME` environment variable (Linux/BSD only), which is
   joined with `lazyincus` to form the directory.

Only non-zero-value keys need to be set explicitly — omitted keys fall back
to the defaults in [config.yml.example](../config.yml.example) (the struct
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
| `sidePanelWidth` | float | `0.3333` | Fraction of screen width used by the Instances side panel. |
| `showBottomLine` | bool | `true` | Show the bottom status/keybinding line. |
| `screenMode` | string | `"normal"` | Initial screen mode: `normal`, `half`, or `full`. |
| `instanceStatusStyle` | string | `"long"` | Instance status display: `long` (full words), `short` (one/two characters), or `icon`. |
| `border` | string | `"rounded"` | Panel border style: `rounded`, `single`, `double`, or `hidden`. |
| `instanceColumns` | []string | `[name, status, type, ipv4, ipv6, snapshots]` | Which columns the Instances panel shows, and in what order. Valid values: `name`, `status`, `type`, `ipv4`, `ipv6`, `snapshots`. Unknown values are ignored; list any subset to hide the rest. |

### Top level

| Key | Type | Default | Meaning |
|---|---|---|---|
| `confirmOnQuit` | bool | `false` | Prompt for confirmation when quitting with `q`/`esc` and no other panel is open. |
| `oS.openCommand` | string | macOS: `open {{filename}}`; Linux: `sh -c "xdg-open {{filename}} >/dev/null"` | Command used to open a file. |
| `oS.openLinkCommand` | string | macOS: `open {{link}}`; Linux: `sh -c "xdg-open {{link}} >/dev/null"` | Command used to open a URL. |
| `ignore` | []string | `[]` | Instances whose name contains any of these substrings are hidden from the list. |

## What's not here (yet)

Ported from lazydocker's config in this MVP: `gui`, `confirmOnQuit`, `oS`,
`ignore`. Deliberately **not** ported (see
[CLAUDE.md](../CLAUDE.md#what-was-intentionally-dropped-for-this-mvp) for
why): `commandTemplates` (docker-compose command templates), `customCommands`,
`bulkCommands`, `stats`/`graphs`, `replacements`, and `logs` (`since`/`tail`/
`timestamps` don't map onto Incus's console-log snapshot model).
