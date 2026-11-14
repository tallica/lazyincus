#!/usr/bin/env bash
# Records a compose file's change marked ↻: c edits the lzi-cfg stack's file,
# the stack and its cache service read changed, and u recreates it. Its fixture,
# the lzi-cfg stack, is built afresh and left behind;
# `demo/changed.sh down` removes it. Needs incus-compose 1.4.0 or later.
# How to run it is demo/README.md.
set -euo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
export REMOTE_A=${REMOTE_A:-site-a}
ROWS=${ROWS:-34} PAUSE=${PAUSE:-2} TYPE_DELAY=${TYPE_DELAY:-0.15} LEAD=${LEAD:-2.5}
# The edit in vim is the story, so time outside lazyincus plays as recorded.
SHELL_SPEED=${SHELL_SPEED:-1} SHELL_HOLD=${SHELL_HOLD:-2.5}
# shellcheck source=demo/lib.sh
source "$(dirname "$0")/lib.sh"
P=lzi-cfg

compose() { (cd "$HERE/$P" && incus-compose "$@"); }

down() {
  local f
  compose down --volumes >/dev/null 2>&1 || true
  # down leaves the pulled images behind, and a project goes only once empty.
  for f in $(incus image list -f csv -c f --project "$P" 2>/dev/null); do
    incus image delete "$f" --project "$P" || true
  done
  incus project delete "$P" 2>/dev/null || true
}

up() { compose up --detach; }

if [[ ${1:-} == down ]]; then down; exit; fi
[[ ${SKIP_FIXTURE:-} ]] || { down >/dev/null 2>&1; up >/dev/null; }

cp -R "$HERE/$P" "$DEMO_HOME/"
demo_home "$REMOTE_A"
printf 'stacks:\n  - %s:%s/%s\n' "$REMOTE_A" "$DEMO_HOME" "$P" >"$CFG/state.yml"
start_app
caption "lazyincus" "a compose file changed, and what up would recreate"
# Scoped to the fixture's project before the recording starts.
wait_screen "Incus v"
steps=$(project_steps "$P")
t send-keys -t app P; sleep 0.5
for ((n = 0; n < steps; n++)); do t send-keys -t app j; sleep 0.1; done
t send-keys -t app Enter
wait_screen "$REMOTE_A/$P)"
t send-keys -t app 1    # Stacks
wait_screen "queue"
start_recording

# --- storyboard ----------------------------------------------------------
key c -- "c: the stack's compose file, in \$EDITOR"
key / -- "find…"
type_text "MAXMEMORY" "…the setting to change"
key Enter -- "there"
key f 6 r 9 l r 6 -- "64mb to 96mb"
type_text ":wq" "save…"
key Enter -- "…and quit"
back_from_shell
wait_screen "↻"
caption "↻" "the stack no longer matches its file"; sleep 3
key 2 -- "Services: which service, by incus-compose's own config hash"
caption "Changed" "the Info tab says so too"; sleep 3
key u -- "u: up, which recreates what's marked ↻ and nothing else"
back_from_shell
caption "cache" "recreated with the new setting: nothing marked"; sleep 4
# -------------------------------------------------------------------------

finish_recording
render
