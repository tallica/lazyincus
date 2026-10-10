#!/usr/bin/env bash
# Records the command palette (ctrl+p) on one remote: going to an item, its
# actions, and the actions with no key of their own. Its fixture, project
# lzi-shop with instances web, db and cache, is built afresh and left behind;
# `demo/palette.sh down` removes it. How to run it is demo/README.md.
set -euo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
export REMOTE_A=${REMOTE_A:-site-a}
COLS=${COLS:-120} ROWS=${ROWS:-32} PAUSE=${PAUSE:-2} TYPE_DELAY=${TYPE_DELAY:-0.15} LEAD=${LEAD:-1.5}
# shellcheck source=demo/lib.sh
source "$(dirname "$0")/lib.sh"
P=lzi-shop

down() {
  local c
  for c in $(incus list --project "$P" -f csv -c n 2>/dev/null); do incus delete -f "$c" --project "$P" || true; done
  incus project delete "$P" 2>/dev/null || true
}

up() {
  incus project create "$P" -c features.networks=false -c features.images=false
  incus profile device add default root disk path=/ pool=default --project "$P"
  incus profile device add default eth0 nic network=incusbr0 name=eth0 --project "$P"
  for c in web db cache; do incus launch images:alpine/edge "$c" --project "$P"; done
  # Opening on addresses rather than a column of zeroes.
  until ! incus list --project "$P" -f csv -c 4 | grep -qvF .; do sleep 1; done
}

# search "text" "caption" — typed slowly enough to watch the palette narrow, then held
search() { TYPE_DELAY=0.25 type_text "$@"; sleep 1; }

if [[ ${1:-} == down ]]; then down; exit; fi
[[ ${SKIP_FIXTURE:-} ]] || { down >/dev/null 2>&1; up >/dev/null; }

demo_home "$REMOTE_A"
start_app
caption "lazyincus" "the command palette"
# Scoped to the fixture's project before the recording starts.
wait_screen "Incus v"
steps=$(project_steps "$P")
t send-keys -t app P; sleep 0.5
for ((n = 0; n < steps; n++)); do t send-keys -t app j; sleep 0.1; done
t send-keys -t app Enter
wait_screen "$REMOTE_A/$P)"
t send-keys -t app 3    # Instances: P pressed this early can hand the focus back to Stacks
start_recording

# --- storyboard ----------------------------------------------------------
key C-p -- "every panel's actions, each naming what it acts on"
sleep 2
search "snap" "type to narrow them"
key C-u -- "clear"
search "db" "…and every list's items, to go to"
key Tab -- "db's own actions"
sleep 2
search "desc" "some have no key: the palette is where they live"
key Enter -- "edit db's description"
type_text "PostgreSQL primary" "…"
key Enter -- "the Info tab shows it under the name"
sleep 2
key C-p -- "again"
search "rename" "rename db"
key Enter -- "…"
key C-u -- "clear the name"
type_text "postgres" "the new one"
key Enter -- "Incus renames only a stopped instance…"
sleep 1
key y -- "…so it's stopped, renamed and started again"
wait_state "$P" postgres RUNNING
caption "ctrl+p" "the command palette"; sleep 4
# -------------------------------------------------------------------------

finish_recording
render
