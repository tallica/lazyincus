# shellcheck shell=bash
# Shared by the recordings in demo/: sourced, not run. A recording sets
# REMOTE_A (and its other remotes) first, then: demo_home, start_app,
# start_recording, its storyboard, finish_recording, render.
# What each setting does is demo/README.md.
export INCUS_REMOTE=$REMOTE_A

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
BIN=${BIN:-$HERE/../lazyincus}
COLS=${COLS:-140}
ROWS=${ROWS:-40}
PAUSE=${PAUSE:-1.2}          # seconds held after each key
TYPE_DELAY=${TYPE_DELAY:-0.08} # seconds between typed characters
LEAD=${LEAD:-4}              # seconds the recording opens on before the first key
SPEED=${SPEED:-1}            # playback speed of the rendered gif/mp4; the daemon waits shrink too
SHELL_SPEED=${SHELL_SPEED:-3} # how much faster time outside lazyincus plays: shells, editors, compose
OUT=${OUT:-$HERE/$(basename "$0" .sh)}
# base16 Tomorrow Night Eighties: background, foreground, then ANSI 0-15
PALETTE=222222,cccccc,666666,f2777a,99cc99,ffcc66,6699cc,cc99cc,66cccc,ffffff,666666,f2777a,99cc99,ffcc66,6699cc,cc99cc,66cccc,ffffff
# a static Regular face: asking for "JetBrains Mono" can land on the lighter variable font
FONT=${FONT:-JetBrainsMono Nerd Font Mono}
SOCK=lzi-demo                # private tmux server: your own tmux is untouched
# Under /tmp, as the editor's file names on screen give the folder away.
CFG=$(mktemp -d /tmp/lzi-demo-config.XXXXXX)
# What the recording shows of this machine: a home of its own, holding the
# stacks, so their paths read ~/…, and an incus config naming the demo's
# remotes alone, so the remotes menu lists nothing else.
DEMO_HOME=$(mktemp -d /tmp/lzi-demo-home.XXXXXX)
REAL_CONF=$(cd "${INCUS_CONF:-$HOME/.config/incus}" && pwd -P)

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

cleanup() { t kill-server 2>/dev/null || true; rm -rf "$CFG" "$DEMO_HOME"; }
trap cleanup EXIT

# demo_home <remote>… — the incus config lazyincus sees: these remotes, the
# first the one it starts on. <name>=<remote> lists a remote as <name>.
demo_home() {
  local arg names=""
  mkdir -p "$DEMO_HOME/incus/servercerts"
  cp "$REAL_CONF/client.crt" "$REAL_CONF/client.key" "$DEMO_HOME/incus/"
  for arg in "$@"; do
    [[ $arg == *=* ]] || arg=$arg=$arg
    cp "$REAL_CONF/servercerts/${arg#*=}.crt" "$DEMO_HOME/incus/servercerts/${arg%%=*}.crt"
    names+="${arg#*=}=${arg%%=*} "
  done
  APP_REMOTE=${1%%=*}
  { echo "default-remote: $APP_REMOTE"; echo "remotes:"
    awk -v names="$names" '
      BEGIN { n = split(names, m, " "); for (i = 1; i <= n; i++) { split(m[i], p, "="); as[p[1]] = p[2] } }
      /^[^ ]/ { k = 0 }
      /^  [^ ]/ { r = $1; sub(/:$/, "", r); k = r in as; if (k) { print "  " as[r] ":"; next } }
      k' "$REAL_CONF/config.yml"
    echo "aliases: {}"; } >"$DEMO_HOME/incus/config.yml"
}

# project_steps <project> — the Projects menu lists "all projects" first, then every project by name
project_steps() {
  incus project list -f csv -c n | sed 's/ (current)//' | LC_ALL=C sort | grep -nx "$1" | cut -d: -f1
}

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
    t send-keys -t app -l "$c"; sleep "$TYPE_DELAY"
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

# wait_screen <text> — until the screen shows it, for keys pressed before recording
wait_screen() {
  local _
  for _ in $(seq 120); do
    t capture-pane -p -t app | grep -qF "$1" && return
    sleep 0.5
  done
  echo "never on screen: $1" >&2; return 1
}

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
# be read at full length. In is the footer, or, while a prompt such as the
# palette's or a filter's takes the footer over, the panels' bottom border
# above it.
watch_screen() {
  local prev="" state last above
  while kill -0 "$REC" 2>/dev/null; do
    { read -r above; read -r last; } < <(t capture-pane -p -t app 2>/dev/null | grep -v '^[[:space:]]*$' | tail -2) || true
    case $last in
      *"Incus v"*|*"Press enter to return"*) state=in ;;
      *) case $above in ╰*|╚*) state=in ;; *) state=out ;; esac ;;
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

# start_app — lazyincus in its own tmux server, with a caption bar, on demo_home's first remote
start_app() {
  t new-session -d -s app -x "$COLS" -y "$ROWS" -c "$DEMO_HOME" \
    "HOME=$DEMO_HOME TMPDIR=/tmp INCUS_CONF=$DEMO_HOME/incus COLORTERM=truecolor CONFIG_DIR=$CFG EDITOR=vim INCUS_REMOTE=$APP_REMOTE $BIN"
  t set -g default-terminal tmux-256color
  t set -as terminal-features ',*:RGB'     # pass the hex selection colour through
  t set -g status-position bottom
  t set -g status-style bg=colour236
  t set -g status-left-length 200
  t set -g status-right ""
  t set -g window-status-format ""
  t set -g window-status-current-format ""
}

# start_recording — attach the recorder as a client of the app's tmux server
start_recording() {
  asciinema rec --overwrite --headless -f asciicast-v2 \
    --window-size "${COLS}x$((ROWS + 1))" \
    -c "env -u TMUX tmux -L $SOCK attach -t app" "$OUT.cast" &
  REC=$!
  REC_START=$(now)
  watch_screen >"$OUT.screens" &
  sleep "$LEAD"
}

# finish_recording — stop before quitting, which would end on tmux's [exited] screen
finish_recording() {
  local cut
  cut=$(awk -v now="$(now)" -v start="$REC_START" 'BEGIN { print now - start - 0.2 }')
  t detach-client -s app 2>/dev/null || true
  wait "$REC" || true
  # What tmux prints as it detaches: a cleared screen and "[detached …]".
  { head -1 "$OUT.cast"
    tail -n +2 "$OUT.cast" | jq -c --argjson cut "$cut" 'select(.[0] < $cut or .[1] != "o")'
  } >"$OUT.cast.tmp" && mv "$OUT.cast.tmp" "$OUT.cast"
  cp "$OUT.cast" "$OUT.raw.cast"    # as recorded, to squeeze again at another speed
  squeeze_shells "$OUT.cast" "$OUT.screens"
  embed_theme "$OUT.cast"
}

render() {
  agg --speed "$SPEED" --theme "$PALETTE" --font-family "$FONT" --font-size 32 --idle-time-limit 6 --last-frame-duration 3 -q "$OUT.cast" "$OUT.gif"
  ffmpeg -y -loglevel error -i "$OUT.gif" -movflags faststart -pix_fmt yuv420p \
    -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2" "$OUT.mp4"
  echo "wrote $OUT.cast $OUT.gif $OUT.mp4"
}
