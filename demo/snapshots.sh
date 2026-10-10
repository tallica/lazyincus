#!/usr/bin/env bash
# Records a snapshot taken, renamed, and made into an instance of its own,
# which is then described and started. Its fixture, project lzi-snap with
# instance blog, is built afresh and left behind; `demo/snapshots.sh down`
# removes it. How to run it is demo/README.md.
set -euo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
export REMOTE_A=${REMOTE_A:-site-a}
ROWS=${ROWS:-34} PAUSE=${PAUSE:-1.7} TYPE_DELAY=${TYPE_DELAY:-0.15} LEAD=${LEAD:-2}
# shellcheck source=demo/lib.sh
source "$(dirname "$0")/lib.sh"
P=lzi-snap

down() {
  local c
  for c in $(incus list --project "$P" -f csv -c n 2>/dev/null); do incus delete -f "$c" --project "$P" || true; done
  incus project delete "$P" 2>/dev/null || true
}

up() {
  incus project create "$P" -c features.networks=false -c features.images=false -c features.profiles=false
  incus launch images:alpine/edge blog --project "$P"
  until ! incus list --project "$P" -f csv -c s4 | grep -q '^RUNNING,$'; do sleep 1; done
}

# search "text" "caption" — typed slowly enough to watch the palette narrow, then held
search() { TYPE_DELAY=0.25 type_text "$@"; sleep 1; }

if [[ ${1:-} == down ]]; then down; exit; fi
[[ ${SKIP_FIXTURE:-} ]] || { down >/dev/null 2>&1; up >/dev/null; }

demo_home "$REMOTE_A"
start_app
caption "lazyincus" "from a snapshot to an instance"
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
key n -- "snapshot blog"
type_text "nightly" "name it"
key Enter -- "taken"
key 4 -- "Snapshots"
key C-p -- "the palette"
search "rename" "rename the snapshot"
key Enter -- "…"
key C-u -- "clear the name"
type_text "before-upgrade" "a better one"
key Enter -- "renamed"
key C-p -- "the palette again"
search "new inst" "a new instance from the snapshot"
key Enter -- "it suggests a name"
key Enter -- "copied in the background…"
wait_state "$P" blog-before-upgrade STOPPED
sleep 1
caption "blog-before-upgrade" "…and the cursor lands on it"; sleep 2.5
key C-p -- "the palette"
search "desc" "describe it"
key Enter -- "…"
type_text "staging copy" "…"
key Enter -- "the Info tab shows it under the name"
sleep 1
key S -- "start it"
wait_state "$P" blog-before-upgrade RUNNING
caption "n, then ctrl+p" "snapshot, rename, new instance from snapshot, edit description"; sleep 4
# -------------------------------------------------------------------------

finish_recording
render
