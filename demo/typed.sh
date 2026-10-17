#!/usr/bin/env bash
# Records the typed commands: `:` running an incus command from the row's
# project, tab completing it from the CLI, ↑ recalling it, and `;` running an
# incus-compose one in front of a service. Its fixture, project lzi-cli with
# instances ledger and ledger-replica, and the lzi-feed stack, is built afresh
# and left behind; `demo/typed.sh down` removes it. How to run it is
# demo/README.md.
set -euo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
export REMOTE_A=${REMOTE_A:-site-a}
COLS=${COLS:-120} ROWS=${ROWS:-32} PAUSE=${PAUSE:-2} TYPE_DELAY=${TYPE_DELAY:-0.15} LEAD=${LEAD:-1.5}
# shellcheck source=demo/lib.sh
source "$(dirname "$0")/lib.sh"
P=lzi-cli
S=lzi-feed

compose() { (cd "$HERE/$S" && incus-compose "$@"); }

down() {
  local c f
  for c in $(incus list --project "$P" -f csv -c n 2>/dev/null); do incus delete -f "$c" --project "$P" || true; done
  incus project delete "$P" 2>/dev/null || true
  compose down --volumes >/dev/null 2>&1 || true
  # down leaves the pulled image behind, and a project goes only once empty.
  for f in $(incus image list -f csv -c f --project "$S" 2>/dev/null); do
    incus image delete "$f" --project "$S" || true
  done
  incus project delete "$S" 2>/dev/null || true
}

up() {
  incus project create "$P" -c features.networks=false -c features.images=false
  incus profile device add default root disk path=/ pool=default --project "$P"
  incus profile device add default eth0 nic network=incusbr0 name=eth0 --project "$P"
  for c in ledger ledger-replica; do incus launch images:alpine/edge "$c" --project "$P"; done
  compose up --detach
}

if [[ ${1:-} == down ]]; then down; exit; fi
[[ ${SKIP_FIXTURE:-} ]] || { down >/dev/null 2>&1; up >/dev/null 2>&1; }

cp -R "$HERE/$S" "$DEMO_HOME/"
demo_home "$REMOTE_A"
printf 'stacks:\n  - %s:%s/%s\n' "$REMOTE_A" "$DEMO_HOME" "$S" >"$CFG/state.yml"
start_app
caption "lazyincus" "typed commands"
# Scoped to the fixture's project before the recording starts.
wait_screen "Incus v"
steps=$(project_steps "$P")
t send-keys -t app P; sleep 0.5
for ((n = 0; n < steps; n++)); do t send-keys -t app j; sleep 0.1; done
t send-keys -t app Enter
wait_screen "$REMOTE_A/$P)"
t send-keys -t app 3    # Instances, on ledger
wait_screen "ledger"
start_recording

# --- storyboard ----------------------------------------------------------
key : -- "an incus command, starting from the row's project"
type_text "sn" "the first letters…"
key Tab -- "tab asks incus itself what comes next"
key Tab -- "a subcommand…"
type_text "cr" "…"
key Tab -- "create"
type_text "le" "an instance…"
key Tab -- "the daemon's names, as far as they agree"
type_text " before-upgrade" "name the snapshot"
key Enter -- "run it, the terminal handed over"
back_from_shell
caption "before-upgrade" "ledger's new snapshot"; sleep 2.5
key : -- "again"
type_text "sn" "start it the same way…"
key Up -- "↑ recalls the line that did"
key Escape -- "never mind"
key 2 -- "Services: the lzi-feed stack's"
key \; -- "an incus-compose command, in the stack's directory"
caption ";" "the service waits after the cursor, for the verb before it"; sleep 2.5
type_text "p" "…"
key Tab -- "incus-compose's verbs: several, listed"
key C-u -- "clear the verb, the service stays"
type_text "res" "…"
key Tab -- "one, finished"
key Enter -- "restart it"
back_from_shell
caption ": ;" "incus and incus-compose commands, with tab and ↑"; sleep 4
# -------------------------------------------------------------------------

finish_recording
render
