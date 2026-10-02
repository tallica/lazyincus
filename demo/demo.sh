#!/usr/bin/env bash
# Records a paced, captioned walkthrough of every lazyincus key, destructive ones
# included, against the throwaway fixture from demo-fixture.sh, on two remotes.
# How to run it, and what it needs, is demo/README.md.
set -euo pipefail
export REMOTE_A=${REMOTE_A:-site-a} REMOTE_B=${REMOTE_B:-site-b}
export INCUS_REMOTE=$REMOTE_A

HERE=$(cd "$(dirname "$0")" && pwd)
BIN=${BIN:-$HERE/../lazyincus}
COLS=${COLS:-140}
ROWS=${ROWS:-40}
PAUSE=${PAUSE:-1.2}          # seconds held after each key
SPEED=${SPEED:-1}            # playback speed of the rendered gif/mp4; the daemon waits shrink too
SHELL_SPEED=${SHELL_SPEED:-3} # how much faster time outside lazyincus plays: shells, editors, compose
OUT=${OUT:-$HERE/demo}
# base16 Tomorrow Night Eighties: background, foreground, then ANSI 0-15
PALETTE=222222,cccccc,666666,f2777a,99cc99,ffcc66,6699cc,cc99cc,66cccc,ffffff,666666,f2777a,99cc99,ffcc66,6699cc,cc99cc,66cccc,ffffff
# a static Regular face: asking for "JetBrains Mono" can land on the lighter variable font
FONT=${FONT:-JetBrainsMono Nerd Font Mono}
SOCK=lzi-demo                # private tmux server: your own tmux is untouched
DEMO_PROJECT=lzi-demo
STACK_PROJECT=lzi-stack
# Under /tmp, as the editor's file names on screen give the folder away.
CFG=$(mktemp -d /tmp/lzi-demo-config.XXXXXX)
# What the recording shows of this machine: a home of its own, holding the
# stacks, so their paths read ~/…, and an incus config naming the two remotes
# alone, so the remotes menu lists nothing else.
DEMO_HOME=$(mktemp -d /tmp/lzi-demo-home.XXXXXX)
REAL_CONF=$(cd "${INCUS_CONF:-$HOME/.config/incus}" && pwd -P)

[[ ${SKIP_FIXTURE:-} ]] || { "$HERE/demo-fixture.sh" down >/dev/null 2>&1; "$HERE/demo-fixture.sh" up >/dev/null; }

cp -R "$HERE/lzi-stack" "$HERE/lzi-edge" "$DEMO_HOME/"
mkdir -p "$DEMO_HOME/incus/servercerts"
cp "$REAL_CONF/client.crt" "$REAL_CONF/client.key" "$DEMO_HOME/incus/"
cp "$REAL_CONF/servercerts/$REMOTE_A.crt" "$REAL_CONF/servercerts/$REMOTE_B.crt" "$DEMO_HOME/incus/servercerts/"
{ echo "default-remote: $REMOTE_A"; echo "remotes:"
  awk -v a="  $REMOTE_A:" -v b="  $REMOTE_B:" '
    /^  [^ ]/ { keep = ($0 == a || $0 == b) } /^[^ ]/ { keep = 0 } keep' "$REAL_CONF/config.yml"
  echo "aliases: {}"; } >"$DEMO_HOME/incus/config.yml"

# Both stacks pinned to their remote, saved as `a` would: no local stack, as
# lazyincus starts outside one, so a switch moves the * and nothing else.
printf 'stacks:\n  - %s:%s/lzi-stack\n' "$REMOTE_A" "$DEMO_HOME" >"$CFG/state.yml"

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
    -e 's/Space/space/g' -e 's/C-\(.\)/ctrl+\1/g' -e 's/Right/→/g' -e 's/Left/←/g'
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

# repeat <key> <times> <caption> — the same key over and over, through a
# menu say, under one caption and without the hold after each
repeat() {
  local n
  caption "$1" "$3"
  sleep 0.4
  for ((n = 0; n < $2; n++)); do t send-keys -t app "$1"; sleep 0.25; done
  sleep "$PAUSE"
}

# pick <name> — filter the focused list down to one row
pick() { key / -- "filter"; type_text "$1" "…to $1"; key Enter -- "keep the filter"; }

# a subprocess (compose verb, exec, editor…) finished: hold its output, then return
back_from_shell() {
  local _
  for _ in $(seq 240); do
    t capture-pane -p -t app | grep -v '^[[:space:]]*$' | tail -1 | grep -q 'Press enter to return' && break
    sleep 0.5
  done
  sleep 0.6
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

now() { perl -MTime::HiRes=time -e 'printf "%.3f\n", time'; }

# watch_screen — while recording, when the screen leaves lazyincus and comes
# back: "<time> in|out" at each change. Out is a shell, an editor or a compose
# command's output, its "Press enter to return" excepted, which stays in to
# be read at full length.
watch_screen() {
  local prev="" state last
  while kill -0 "$REC" 2>/dev/null; do
    last=$(t capture-pane -p -t app 2>/dev/null | grep -v '^[[:space:]]*$' | tail -1) || true
    case $last in
      *"Incus v"*|*"Press enter to return"*) state=in ;;
      *) state=out ;;
    esac
    [[ $state != "$prev" ]] && echo "$(now) $state"
    prev=$state
    sleep 0.25
  done
}

# squeeze_shells <cast> <screens> — play the time out of lazyincus, as
# watch_screen saw it, SHELL_SPEED times faster, no pause in it longer than
# 0.4s. The keys were sent at the pace the commands ran at; only playback
# changes.
squeeze_shells() {
  local spans
  spans=$(awk -v start="$REC_START" '
    $2 == "out" { from = $1 - start }
    $2 == "in" && from != "" { printf "%s[%s,%s]", (n++ ? "," : ""), from, $1 - start; from = "" }
    END { if (from != "") printf "%s[%s,1e9]", (n ? "," : ""), from }' "$2")

  { head -1 "$1"
    tail -n +2 "$1" | jq -c -n --argjson f "$SHELL_SPEED" --argjson spans "[$spans]" '
      foreach inputs as $e ({prev: 0, at: 0};
        ($e[0] - .prev) as $d
        | ([$spans[] | select(.[0] <= $e[0] and $e[0] <= .[1])] | length > 0) as $out
        | .at += (if $out then [$d / $f, 0.4] | min else $d end)
        | .prev = $e[0];
        [.at, $e[1], $e[2]])'
  } >"$1.tmp" && mv "$1.tmp" "$1"
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

cleanup() { t kill-server 2>/dev/null || true; rm -rf "$CFG" "$DEMO_HOME"; }
trap cleanup EXIT

# The Projects menu lists "all projects" first, then every project by name.
project_steps=$(incus project list -f csv -c n | sed 's/ (current)//' | LC_ALL=C sort | grep -nx "$DEMO_PROJECT" | cut -d: -f1)
# The Remotes menu lists the instance remotes by name, from its first row.
remote_steps=$(INCUS_CONF=$DEMO_HOME/incus incus remote list -f csv -c npP |
  awk -F, '$2 == "incus" && $3 == "NO" { sub(/ \(current\)/, "", $1); print $1 }' |
  LC_ALL=C sort | grep -nx "$REMOTE_B" | cut -d: -f1)
remote_steps=$((remote_steps - 1))
# Stacks lists each remote's together, by name: from lzi-edge to lzi-stack.
to_stack_a=j; [[ $REMOTE_A < $REMOTE_B ]] && to_stack_a=k

# --- the app, in its own tmux server with a caption bar ------------------
t new-session -d -s app -x "$COLS" -y "$ROWS" -c "$DEMO_HOME" \
  "HOME=$DEMO_HOME TMPDIR=/tmp INCUS_CONF=$DEMO_HOME/incus COLORTERM=truecolor CONFIG_DIR=$CFG EDITOR=vim INCUS_REMOTE=$REMOTE_A $BIN"
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
REC_START=$(now)
watch_screen >"$OUT.screens" &
sleep 4

# --- storyboard ----------------------------------------------------------
chapter "Getting around"
key 2 -- "Services: the stack's, from compose.yaml"
key j -- "move down"
key j -- "a replicated service lists its instances"
key k k -- "move up"
key Tab -- "next side panel"
key Tab -- "next side panel"
key Tab -- "next side panel"
key BTab -- "previous side panel"
key BTab -- "…Instances, a long list"
key '=' -- "expand the focused panel, collapse the rest"
key BTab -- "the room follows focus"
key Tab -- "…back"
key '=' -- "split the column evenly again"

chapter "Projects"
key P -- "switch Incus project"
repeat j "$project_steps" "…down to $DEMO_PROJECT"
key Enter -- "scope everything to $DEMO_PROJECT"

chapter "The main panel"
key 3 -- "Instances"
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
key 3 -- "Instances"
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
key 5 -- "Images"
key u -- "instances using this image"
key Escape -- "back"
key D -- "prune images…"
key j -- "every unused one"
key Enter -- "names each"
key y -- "…confirmed"
key 5 -- "again: Volumes"
pick demo-data
key d -- "delete the custom volume…"
key y -- "…confirmed"
key Escape -- "clear the filter"
key 5 -- "Networks"
key e -- "the host's own interfaces too"
key e -- "managed networks only"
pick lzidemo0
key d -- "delete the network…"
key y -- "…confirmed"
key Escape -- "clear the filter"
key 5 -- "Profiles"
pick demo-unused
key d -- "delete a profile nothing uses…"
key y -- "…confirmed"
key Escape -- "clear the filter"

chapter "A compose stack"
key 1 -- "Stacks: compose projects, each saved on its remote"
key Enter -- "its Info: endpoints, usage, drift"
key ] -- "every service's logs"
key ] -- "the whole compose config"
key [ [ -- "back to Info"
key Escape -- "back to the list"
key 2 -- "its services"
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
key 1 -- "Stacks: the same verbs, for the whole stack"
key u -- "bring it all up"; back_from_shell

chapter "Two remotes"
key R -- "the remotes menu"
repeat j "$remote_steps" "…down to $REMOTE_B"
key Enter -- "no stack on $REMOTE_B: Stacks and Services fold away"
sleep 3
key 1 -- "focus Stacks and Instances folds instead"
key a -- "add a stack…"
type_text "$REMOTE_B:~/lzi-edge" "…pinned to a remote, the way incus names it"
key Enter -- "a remote column, * on the one the screen is on"
sleep 3
key Enter -- "every Info tab starts with where it lives"
key Escape -- "back to the list"
key 2 -- "its services"
key Enter -- "remote, project, stack, service"
key Escape -- "back to the list"
key 1 -- "Stacks"
key "$to_stack_a" -- "lzi-stack, still on $REMOTE_A"
key 2 -- "its services, read from $REMOTE_A"
key s -- "stop… the prompt names the remote"
key y -- "…confirmed"; back_from_shell
key S -- "start it"; back_from_shell
key 1 -- "Stacks"
key Space -- "switch the whole screen back to $REMOTE_A"
sleep 3
key 3 -- "$REMOTE_A's instances"
key 1 -- "the list stays put; * follows"
sleep 2

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
cp "$OUT.cast" "$OUT.raw.cast"    # as recorded, to squeeze again at another speed
squeeze_shells "$OUT.cast" "$OUT.screens"
embed_theme "$OUT.cast"

agg --speed "$SPEED" --theme "$PALETTE" --font-family "$FONT" --font-size 32 --idle-time-limit 6 --last-frame-duration 3 -q "$OUT.cast" "$OUT.gif"
ffmpeg -y -loglevel error -i "$OUT.gif" -movflags faststart -pix_fmt yuv420p \
  -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2" "$OUT.mp4"
echo "wrote $OUT.cast $OUT.gif $OUT.mp4"
"$HERE/highlights.sh" "$OUT.cast"
