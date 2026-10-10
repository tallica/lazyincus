# The demo

The walkthrough linked from the [README](../README.md) is recorded here: a
script presses every key against a throwaway fixture on two Incus remotes,
captioning each, and writes an asciicast, a gif and an mp4, then cuts the
short gif the README shows from it. Beside it are short recordings of one
feature each, to link from its CHANGELOG entry.

| File | What it does |
|---|---|
| `demo.sh` | Records the walkthrough: the storyboard of keys and captions |
| `demo-fixture.sh` | Builds and tears down what the walkthrough acts on |
| `highlights.sh` | Cuts `docs/highlights.gif` from the recording |
| `lzi-stack/`, `lzi-edge/` | The two compose stacks, one per remote |
| `palette.sh` | Records the command palette, with a fixture of its own |
| `lib.sh` | What the recordings share: the tmux server, the keys and captions, the recorder |

## Before you start

- **Two disposable Incus daemons**, each added as a remote of the `incus`
  CLI. The walkthrough stops, deletes and recreates things on both, and the
  fixture deletes the projects it uses before building them again - don't
  point it at a daemon you care about. The defaults are remotes named
  `site-a` and `site-b`.
- On each: a `default` storage pool, the `incusbr0` network, and access to
  `images:` and docker.io for the images the fixture pulls.
- The tools:

  ```sh
  brew install asciinema agg ffmpeg jq tmux incus-compose
  ```

- The JetBrainsMono Nerd Font Mono font, for the gifs; any other through
  `FONT`.
- A lazyincus binary. The released one keeps the footer's version clean:

  ```sh
  gh release download v0.13.0 -R tallica/lazyincus \
    -p 'lazyincus_0.13.0_darwin_arm64.tar.gz' -O - | tar xz -C /tmp lazyincus
  ```

  or `make build` for the checkout's own, which `demo.sh` uses by default.

## Recording

From the repository root:

```sh
BIN=/tmp/lazyincus REMOTE_A=site-a REMOTE_B=site-b demo/demo.sh
```

It runs on its own for about seven minutes, the recording itself playing
in under six: the fixture is torn down and
built again on both remotes, then the walkthrough plays in a private tmux
server - your own tmux is untouched - while asciinema records it. Leave it
be until it prints what it wrote:

- `demo/demo.cast`, the recording, which is what gets uploaded
- `demo/demo.gif` and `demo/demo.mp4`, rendered from it
- `docs/highlights.gif`, the README's

The first three are ignored by git, as is `demo/demo.screens`, the log
the next part comes from, and `demo/demo.raw.cast`, the recording before
it; the highlights gif is committed.

Time spent outside lazyincus - a shell in an instance, an editor, a
compose command's output - plays faster than it was recorded: while
recording, the screen is checked four times a second for lazyincus's
footer, and the spans without it play `SHELL_SPEED` times faster, no pause
in them longer than 0.4s. The keys still went at the pace the commands
ran at, so nothing is skipped; only the playback is quicker. A command's
"Press enter to return" plays at full length, for its output to be read.

What the recording shows of the machine it's made on is kept to the demo's
own: lazyincus runs with an incus config holding the two remotes alone, so
the remotes menu lists nothing else, and a home of its own under `/tmp`,
holding copies of the two stacks, so their paths read `~/lzi-stack`.

### Settings

| Variable | Default | |
|---|---|---|
| `REMOTE_A`, `REMOTE_B` | `site-a`, `site-b` | The two remotes; the session starts on the first |
| `BIN` | `./lazyincus` | The lazyincus binary to record |
| `COLS`, `ROWS` | `140`, `40` | The terminal's size |
| `PAUSE` | `1.2` | Seconds held after each key |
| `TYPE_DELAY` | `0.08` | Seconds between typed characters |
| `LEAD` | `4` | Seconds the recording opens on before the first key |
| `SPEED` | `1` | Playback speed of the rendered gif and mp4 |
| `SHELL_SPEED` | `3` | How much faster time outside lazyincus plays, in the cast too |
| `FONT` | JetBrainsMono Nerd Font Mono | The gifs' font |
| `OUT` | `demo/demo` | Where the cast, gif and mp4 go, less the extension |
| `SKIP_FIXTURE` | unset | Set to record against the fixture as it stands |

`SKIP_FIXTURE` only works on a fixture nothing has run against since it
was built: the walkthrough deletes some of it on the way.

## Publishing

Upload the cast, which makes it public:

```sh
asciinema upload demo/demo.cast
```

Then point both links at the top of the [README](../README.md) at the new
recording, and commit `docs/highlights.gif` with any change to the scripts.

## Short recordings

One feature each, under a minute, on one remote, `REMOTE_A`:

```sh
REMOTE_A=site-a demo/palette.sh
```

It writes `demo/palette.cast`, `.gif` and `.mp4`, and takes the settings
above at a slower default pace, to be read rather than skimmed: a size of
`120`×`32`, `PAUSE` `2`, `TYPE_DELAY` `0.15` and `LEAD` `1.5`. It builds its own fixture, project
`lzi-shop`, which it leaves behind, renamed as the recording left it;
`demo/palette.sh down` removes it. The project is chosen before the
recording starts, so it opens on the feature.

Upload the cast the same way, then link it at the end of the CHANGELOG
entry it shows, `([demo](https://asciinema.org/a/…))` - which carries it
into the release notes - and from wherever the feature is documented: its
key's row in the [README](../README.md), or its section of
[docs/Panels.md](../docs/Panels.md).

A new one is a copy of `palette.sh`: its fixture's `up` and `down`, and
its storyboard.

## The fixture

`demo-fixture.sh up | down`, which `demo.sh` runs itself, reading
`REMOTE_A` and `REMOTE_B`:

- **On `REMOTE_A`:** project `lzi-demo` with instances `alpha`, `beta` and
  a stopped `gamma`, snapshots, a custom volume with a snapshot, a published
  image, an unused profile, network `lzidemo0`, and the `lzi-stack` compose
  stack, both of its services healthchecked, so their health shows, and
  each of its instances snapshotted, so the Snapshots panel has something
  to show for them.
- **On `REMOTE_B`:** the `lzi-edge` compose stack.
- **On both:** a snapshot of `ic-healthd`, incus-compose's health daemon,
  for the Snapshots panel to show when the cursor lands on it.

`down` removes all of it from both remotes, and whatever an older fixture
left there, so a remote cloned from the other starts clean too.

## Changing the walkthrough

The storyboard is the block under `# --- storyboard ---` in `demo.sh`,
one line a key:

```sh
chapter "Two remotes"                       # a title card
key s -- "stop… the prompt names the remote"   # tmux key names, then the caption
type_text "site-b:~/lzi-edge" "…pinned to a remote"
back_from_shell                             # wait for a subprocess, then return
wait_state lzi-demo beta STOPPED            # wait for the daemon
```

`highlights.sh` cuts its clips at captions, not at times, each with a
length in seconds, so a recording that runs faster or slower still cuts in
the same places - but a caption renamed in `demo.sh` has to be renamed in
its `CLIPS` too. To recut the gif from an existing recording:

```sh
demo/highlights.sh demo/demo.cast docs/highlights.gif
```

Anything that changes what's on screen - a panel's number, a key - is
worth checking against the storyboard, which presses keys blind: a key
that now does something else records a demo of the wrong thing.

## When it goes wrong

- **It stops partway.** The fixture is left as the walkthrough had it;
  run `demo.sh` again without `SKIP_FIXTURE`, which rebuilds it.
- **A caption doesn't match the screen.** The walkthrough doesn't look,
  it presses: watch the recording, or read the live screen from another
  terminal with `tmux -L lzi-demo capture-pane -p -t app` - attaching
  would resize what's being recorded. The caption bar is tmux's status
  line, which `capture-pane` leaves out:
  `tmux -L lzi-demo show -gv status-left` reads it.
- **`highlights.sh` says a caption isn't found.** A clip names a caption
  the storyboard no longer has.
