#!/usr/bin/env bash
# Records a compose stack's backups: B to the Backups tab, a new backup,
# verify, and the volumes holding them. Its fixture is the lzi-notes stack
# with one backup already taken, built afresh and left behind;
# `demo/backups.sh down` removes it. How to run it is demo/README.md.
set -euo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
export REMOTE_A=${REMOTE_A:-site-a}
ROWS=${ROWS:-34} PAUSE=${PAUSE:-1.8} TYPE_DELAY=${TYPE_DELAY:-0.15} LEAD=${LEAD:-2.5}
# shellcheck source=demo/lib.sh
source "$(dirname "$0")/lib.sh"
P=lzi-notes

compose() { (cd "$HERE/$P" && incus-compose "$@"); }

down() {
  local project f
  compose backup delete --keep-last 0 >/dev/null 2>&1 || true
  compose down --volumes >/dev/null 2>&1 || true
  # A project goes only once empty: down leaves the pulled image behind.
  for project in "$P-backup" "$P"; do
    for f in $(incus image list -f csv -c f --project "$project" 2>/dev/null); do
      incus image delete "$f" --project "$project" || true
    done
    incus project delete "$project" 2>/dev/null || true
  done
}

up() {
  compose up --detach
  compose backup create --name nightly --live
}

if [[ ${1:-} == down ]]; then down; exit; fi
[[ ${SKIP_FIXTURE:-} ]] || { down >/dev/null 2>&1; up >/dev/null 2>&1; }

cp -R "$HERE/$P" "$DEMO_HOME/"
demo_home "$REMOTE_A"
printf 'stacks:\n  - %s:%s/%s\n' "$REMOTE_A" "$DEMO_HOME" "$P" >"$CFG/state.yml"
start_app
caption "lazyincus" "a compose stack's backups"
wait_screen "Incus v"
start_recording

# --- storyboard ----------------------------------------------------------
key B -- "the stack's backups, from Stacks or Services"
key n -- "back it up"
type_text "before-upgrade" "name it"
key Enter -- "stop the stack for it, or snapshot it live"
key Enter -- "stopped, snapshotted, started again"
back_from_shell
caption "before-upgrade" "the cursor on the new backup"; sleep 2.5
key v -- "verify: is every restore point there?"
sleep 2
key Enter -- "each volume, beside the one holding its restore points"
sleep 1.5
key Escape -- "back"
key 5 5 -- "Volumes"
pick "$P"
caption "backup" "the volumes holding them, marked rather than unused"; sleep 3
caption "B" "incus-compose backup: n, v, r restores, d / D delete or prune"; sleep 4
# -------------------------------------------------------------------------

finish_recording
render
