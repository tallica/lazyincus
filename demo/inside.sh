#!/usr/bin/env bash
# Records working with what's inside an instance: its ports on the Info
# tab, a file edited in $EDITOR with F, and the main panel searched with /.
# Its fixture, project lzi-web with instance front, is built afresh and left
# behind; `demo/inside.sh down` removes it. How to run it is demo/README.md.
set -euo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
export REMOTE_A=${REMOTE_A:-site-a}
ROWS=${ROWS:-34} PAUSE=${PAUSE:-1.8} TYPE_DELAY=${TYPE_DELAY:-0.15} LEAD=${LEAD:-2}
# The editor is the story here: played as recorded.
SHELL_SPEED=${SHELL_SPEED:-1} SHELL_HOLD=${SHELL_HOLD:-3}
# shellcheck source=demo/lib.sh
source "$(dirname "$0")/lib.sh"
P=lzi-web

down() {
  local c
  for c in $(incus list --project "$P" -f csv -c n 2>/dev/null); do incus delete -f "$c" --project "$P" || true; done
  incus project delete "$P" 2>/dev/null || true
}

up() {
  incus project create "$P" -c features.networks=false -c features.images=false -c features.profiles=false
  incus init images:alpine/edge front --project "$P"
  incus config set front description="the shop's front end" --property --project "$P"
  incus config device add front http proxy listen=tcp:0.0.0.0:8088 connect=tcp:127.0.0.1:80 --project "$P"
  incus start front --project "$P"
  until ! incus list --project "$P" -f csv -c s4 | grep -q '^RUNNING,$'; do sleep 1; done
}

if [[ ${1:-} == down ]]; then down; exit; fi
[[ ${SKIP_FIXTURE:-} ]] || { down >/dev/null 2>&1; up >/dev/null; }

demo_home "$REMOTE_A"
start_app
caption "lazyincus" "inside an instance"
# Scoped to the fixture's project before the recording starts.
wait_screen "Incus v"
steps=$(project_steps "$P")
t send-keys -t app P; sleep 0.5
for ((n = 0; n < steps; n++)); do t send-keys -t app j; sleep 0.1; done
t send-keys -t app Enter
wait_screen "$REMOTE_A/$P)"
t send-keys -t app 3    # Instances: P pressed this early can hand the focus back to Stacks
caption "Info" "the description under its name, and its proxy devices as Ports"
start_recording
sleep 1.5

# --- storyboard ----------------------------------------------------------
key F -- "edit a file inside it, in \$EDITOR"
type_text "etc/motd" "which one"
key Enter -- "pulled out and opened"
sleep 1
key g g d G -- "replace what's there"
key i -- "…"
type_text "Welcome. Mind the shop." "…"
key Escape -- "…"
type_text ":wq" "save and quit"
key Enter -- "pushed back into the instance"
back_from_shell
key E -- "a shell, to check"
type_text "cat /etc/motd" "…"
key Enter -- "there it is"
type_text "exit" "…"
key Enter -- "…"
back_from_shell
key Enter -- "the main panel"
key ] ] -- "the Config tab"
key / -- "search it"
type_text "volatile" "…every match highlighted"
key Enter -- "…"
key n -- "next"
key n -- "next"
key N -- "previous"
key Escape -- "clear"
caption "F  /" "edit a file inside an instance; search the main panel"; sleep 4
# -------------------------------------------------------------------------

finish_recording
render
