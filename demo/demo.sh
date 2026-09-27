#!/usr/bin/env bash
# Records a paced, captioned walkthrough of every lazyincus key, destructive ones
# included, against the throwaway fixture from demo-fixture.sh.
#   brew install asciinema agg ffmpeg jq tmux incus-compose
#   make build && INCUS_REMOTE=<remote> demo/demo.sh   -> demo/demo.{cast,gif,mp4}, docs/highlights.gif
set -euo pipefail
: "${INCUS_REMOTE:?set INCUS_REMOTE to the demo daemon}"
export INCUS_REMOTE

HERE=$(cd "$(dirname "$0")" && pwd)
BIN=${BIN:-$HERE/../lazyincus}
COLS=${COLS:-140}
ROWS=${ROWS:-40}
PAUSE=${PAUSE:-1.2}          # seconds held after each key
SPEED=${SPEED:-1}            # playback speed of the rendered gif/mp4; the daemon waits shrink too
OUT=${OUT:-$HERE/demo}
# base16 Tomorrow Night Eighties: background, foreground, then ANSI 0-15
PALETTE=222222,cccccc,666666,f2777a,99cc99,ffcc66,6699cc,cc99cc,66cccc,ffffff,666666,f2777a,99cc99,ffcc66,6699cc,cc99cc,66cccc,ffffff
# a static Regular face: asking for "JetBrains Mono" can land on the lighter variable font
FONT=${FONT:-JetBrainsMono Nerd Font Mono}
SOCK=lzi-demo                # private tmux server: your own tmux is untouched
DEMO_PROJECT=lzi-demo
STACK_PROJECT=lzi-stack
CFG=$(mktemp -d)

[[ ${SKIP_FIXTURE:-} ]] || { "$HERE/demo-fixture.sh" down >/dev/null 2>&1; "$HERE/demo-fixture.sh" up >/dev/null; }

cat >"$CFG/config.yml" <<'EOF'
gui:
  expandFocusedSidePanel: false
  border: rounded
  theme:
    selectedLineBgColor: ["#515151"]
oS:
  copyToClipboardCommand: dd of=/dev/null status=none
EOF

t() { env -u TMUX tmux -L "$SOCK" -f /dev/null "$@"; }

caption() {
  local k=${1//\#/##} l=${2//\#/##}      # the status line reads # and % as formats
  t set -g status-left "#[bg=colour214,fg=colour16,bold]  ${k//%/%%}  #[bg=colour236,fg=colour255,nobold]  ${l//%/%%} "
}

pretty() {
  sed -e 's/BTab/shift+tab/g' -e 's/Tab/tab/g' -e 's/Escape/esc/g' -e 's/Enter/enter/g' \
    -e 's/C-\(.\)/ctrl+\1/g' -e 's/Right/→/g' -e 's/Left/←/g'
}

# key <tmux key names> -- <caption>
key() {
  local keys=() label
  while [[ $1 != -- ]]; do keys+=("$1"); shift; done; shift
  label=$1
  caption "$(echo "${keys[*]}" | pretty)" "$label"
  sleep 0.4                  # the caption lands before the key does
  for k in "${keys[@]}"; do t send-keys -t app "$k"; sleep 0.2; done
  sleep "$PAUSE"
}

# type_text "abc" "caption" — one character at a time
type_text() {
  caption "$1" "$2"
  local i c
  for ((i = 0; i < ${#1}; i++)); do
    c=${1:i:1}; [[ $c == ';' ]] && c='\;'    # a bare ; ends the tmux command
    t send-keys -t app -l "$c"; sleep 0.08
  done
  sleep "$PAUSE"
}

chapter() { caption "▶" "$1"; sleep 1.8; }

# pick <name> — filter the focused list down to one row
pick() { key / -- "filter"; type_text "$1" "…to $1"; key Enter -- "keep the filter"; }

# a subprocess (compose verb, exec, editor…) finished: hold its output, then return
back_from_shell() {
  local _
  for _ in $(seq 240); do
    t capture-pane -p -t app | grep -v '^[[:space:]]*$' | tail -1 | grep -q 'Press enter to return' && break
    sleep 0.5
  done
  sleep "$PAUSE"
  key Enter -- "back to lazyincus"
}

# wait_state <project> <instance> <STATE|gone> — let the daemon catch up before moving on
wait_state() {
  local _ s
  for _ in $(seq 120); do
    s=$(incus list --project "$1" -f csv -c s "^$2\$" 2>/dev/null)
    [[ ${s:-gone} == "$3" ]] && break
    sleep 0.5
  done
  sleep 0.5
}

# embed_theme <cast> — write $PALETTE into the header; recorded headless under tmux, it carries none
embed_theme() {
  local c p
  IFS=, read -ra c <<<"$PALETTE"
  p=$(printf ':#%s' "${c[@]:2}")
  { head -1 "$1" | jq -c --arg bg "#${c[0]}" --arg fg "#${c[1]}" --arg p "${p:1}" \
      '.theme = {fg: $fg, bg: $bg, palette: $p}'
    tail -n +2 "$1"; } >"$1.tmp" && mv "$1.tmp" "$1"
}

cleanup() { t kill-server 2>/dev/null || true; rm -rf "$CFG"; }
trap cleanup EXIT

# The Projects menu lists "all projects" first, then every project by name.
project_steps=$(incus project list -f csv -c n | sed 's/ (current)//' | LC_ALL=C sort | grep -nx "$DEMO_PROJECT" | cut -d: -f1)

# --- the app, in its own tmux server with a caption bar ------------------
t new-session -d -s app -x "$COLS" -y "$ROWS" -c "$HERE/lzi-stack" \
  "COLORTERM=truecolor CONFIG_DIR=$CFG EDITOR=vim INCUS_REMOTE=$INCUS_REMOTE $BIN"
t set -g default-terminal tmux-256color
t set -as terminal-features ',*:RGB'     # pass the hex selection colour through
t set -g status-position bottom
t set -g status-style bg=colour236
t set -g status-left-length 200
t set -g status-right ""
t set -g window-status-format ""
t set -g window-status-current-format ""
caption "lazyincus" "a terminal UI for Incus"

# --- the recorder: attaches a client to that server ----------------------
asciinema rec --overwrite --headless -f asciicast-v2 \
  --window-size "${COLS}x$((ROWS + 1))" \
  -c "env -u TMUX tmux -L $SOCK attach -t app" "$OUT.cast" &
REC=$!
sleep 4

# --- storyboard ----------------------------------------------------------
chapter "Getting around"
key j -- "move down"
key j -- "a replicated service lists its instances"
key k k -- "move up"
key Tab -- "next side panel"
key Tab -- "next side panel"
key Tab -- "next side panel"
key BTab -- "previous side panel"
key '=' -- "expand the focused panel, collapse the rest"
key Tab -- "the room follows focus"
key BTab -- "…back"
key '=' -- "split the column evenly again"

chapter "Projects"
key P -- "switch Incus project"
for ((n = 0; n < project_steps; n++)); do key j -- "…pick one"; done
key Enter -- "scope everything to $DEMO_PROJECT"

chapter "The main panel"
key 2 -- "Instances"
key Enter -- "focus the main panel"
key ] -- "Logs"
key ] -- "Config"
key ] -- "Env"
key ] -- "Top — live processes"
key [ -- "previous tab"
key m -- "straight to Logs"
key Escape -- "back to the list"

chapter "Instance lifecycle"
key j -- "beta"
key s -- "stop…"
key y -- "…confirmed"; wait_state $DEMO_PROJECT beta STOPPED
key S -- "start"; wait_state $DEMO_PROJECT beta RUNNING
key p -- "pause (freeze)"; wait_state $DEMO_PROJECT beta FROZEN
key p -- "unpause"; wait_state $DEMO_PROJECT beta RUNNING
key r -- "restart"; wait_state $DEMO_PROJECT beta RUNNING
key j -- "gamma, stopped"
key S -- "start it"; wait_state $DEMO_PROJECT gamma RUNNING
key d -- "delete…"
key y -- "…confirmed"
key y -- "it's running — stop it and delete anyway"; wait_state $DEMO_PROJECT gamma gone

chapter "Snapshots"
key k k -- "alpha"
key n -- "new snapshot"
type_text "pre-demo" "name it"
key Tab -- "to the options"
key Right Right -- "expires in 7 days"
key Enter -- "create"
key r -- "restore alpha to it…"
key y -- "…confirmed"
key d -- "delete the snapshot…"
key y -- "…confirmed"
key e -- "every instance's snapshots"
key e -- "back to alpha's"

chapter "Into the instance"
key 2 -- "Instances"
key E -- "exec a shell"
type_text "hostname; cat /etc/alpine-release" "run something"
key Enter -- "run it"
type_text "exit" "leave the shell"
key Enter -- "exit"
back_from_shell
key a -- "attach to the console"
key Enter -- "wake the login prompt"
key C-a q -- "detach"
back_from_shell
key c -- "edit its config in \$EDITOR"
key j j j j j j j j -- "incus config edit"
type_text ":q!" "leave unchanged"
key Enter -- "quit the editor"
back_from_shell
key y -- "copy…"
key j -- "the IPv4 address"
key Enter -- "copied to the clipboard"

chapter "Resources"
key 4 -- "Images"
key u -- "instances using this image"
key Escape -- "back"
key D -- "prune images…"
key j -- "every unused one"
key Enter -- "names each"
key y -- "…confirmed"
key 4 -- "again: Volumes"
pick demo-data
key d -- "delete the custom volume…"
key y -- "…confirmed"
key Escape -- "clear the filter"
key 4 -- "Networks"
key e -- "the host's own interfaces too"
key e -- "managed networks only"
pick lzidemo0
key d -- "delete the network…"
key y -- "…confirmed"
key Escape -- "clear the filter"
key 4 -- "Profiles"
pick demo-unused
key d -- "delete a profile nothing uses…"
key y -- "…confirmed"
key Escape -- "clear the filter"

chapter "A compose stack"
key 1 -- "Services, from compose.yaml"
key s -- "stop the cache service…"
key y -- "…confirmed"; back_from_shell
key S -- "start it"; back_from_shell
key j -- "web: two replicas"
key p -- "pause the service"; back_from_shell
key p -- "unpause"; back_from_shell
key r -- "restart"; back_from_shell
key j -- "one replica"
key s -- "stop just this replica…"
key y -- "…confirmed"; wait_state $STACK_PROJECT web-1 STOPPED
key k -- "the service"
key u -- "bring it up again"; back_from_shell
key g -- "pull its image"; back_from_shell
key U -- "pull and recreate…"
key y -- "…confirmed"; back_from_shell
key d -- "bring it down…"
key Enter -- "down…"
key y -- "…confirmed"; back_from_shell
key C -- "the same verbs for the whole stack"
key Enter -- "bring up"; back_from_shell

chapter "Make it yours"
key O -- "edit lazyincus's config"
type_text ":%s/rounded/double/" "change the border"
key Enter -- "…"
type_text ":wq" "save"
key Enter -- "applied without a restart"
back_from_shell
key + -- "half-screen mode"
key + -- "full-screen mode"
key _ -- "back"
key _ -- "normal"
key '?' -- "every keybinding, for where you are"
sleep 2
key Escape -- "close"
caption q "quit"; sleep 2      # stop recording here: quitting ends on tmux's [exited] screen
# -------------------------------------------------------------------------

t detach-client -s app 2>/dev/null || true
wait "$REC" || true
embed_theme "$OUT.cast"

agg --speed "$SPEED" --theme "$PALETTE" --font-family "$FONT" --font-size 32 --idle-time-limit 6 --last-frame-duration 3 -q "$OUT.cast" "$OUT.gif"
ffmpeg -y -loglevel error -i "$OUT.gif" -movflags faststart -pix_fmt yuv420p \
  -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2" "$OUT.mp4"
echo "wrote $OUT.cast $OUT.gif $OUT.mp4"
"$HERE/highlights.sh" "$OUT.cast"
