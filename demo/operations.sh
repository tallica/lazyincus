#!/usr/bin/env bash
# Records W's operations and warnings on one remote: a warning acknowledged,
# a long command cancelled, and a failed start, the last two set off from
# outside lazyincus. Its fixture, project lzi-ops and network lzidemo1, is
# built afresh and left behind; `demo/operations.sh down` removes it. How
# to run it is demo/README.md.
set -euo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
export REMOTE_A=${REMOTE_A:-site-a}
ROWS=${ROWS:-34} PAUSE=${PAUSE:-2} TYPE_DELAY=${TYPE_DELAY:-0.15} LEAD=${LEAD:-2.5}
# shellcheck source=demo/lib.sh
source "$(dirname "$0")/lib.sh"
P=lzi-ops
NET=lzidemo1

down() {
  local c
  for c in $(incus list --project "$P" -f csv -c n 2>/dev/null); do incus delete -f "$c" --project "$P" || true; done
  for f in $(incus image list --project "$P" -f csv -c f 2>/dev/null); do incus image delete "$f" --project "$P" || true; done
  incus project delete "$P" 2>/dev/null || true
  incus network delete "$NET" 2>/dev/null || true    # and the warning it raised
}

up() {
  incus project create "$P" -c features.networks=false -c features.images=false -c features.profiles=false
  # The daemon warns of raw.dnsmasq when the network starts.
  incus network create "$NET" ipv4.address=10.78.0.1/24 ipv6.address=none raw.dnsmasq=log-queries
  incus launch images:alpine/edge ingest --project "$P"
  incus launch images:alpine/edge scheduler --project "$P"
  # Fails to start: incusd itself listens on 8443.
  incus init images:alpine/edge report --project "$P"
  incus config device add report web proxy listen=tcp:0.0.0.0:8443 connect=tcp:127.0.0.1:80 --project "$P"
  until ! incus list --project "$P" -f csv -c s4 | grep -q '^RUNNING,$'; do sleep 1; done
}

if [[ ${1:-} == down ]]; then down; exit; fi
[[ ${SKIP_FIXTURE:-} ]] || { down >/dev/null 2>&1; up >/dev/null; }

demo_home "$REMOTE_A"
start_app
caption "lazyincus" "what the daemon is doing"
# Scoped to the fixture's project before the recording starts.
wait_screen "Incus v"
steps=$(project_steps "$P")
t send-keys -t app P; sleep 0.5
for ((n = 0; n < steps; n++)); do t send-keys -t app j; sleep 0.1; done
t send-keys -t app Enter
wait_screen "$REMOTE_A/$P)"
t send-keys -t app 3    # Instances: P pressed this early can hand the focus back to Stacks
caption "warnings" "the footer counts the ones nobody has acknowledged"
start_recording

# --- storyboard ----------------------------------------------------------
key W -- "the remote's warnings, new ones first"
key a -- "acknowledge one: the daemon keeps it, so every client agrees"
key Escape -- "close"
# An exec, rather than a download: Incus finishes a download it says it cancelled.
incus exec ingest --project "$P" -- sleep 600 >/dev/null 2>&1 &
JOB=$!
caption "elsewhere" "someone runs a long job in ingest: the footer counts it"
wait_screen "1 running"; sleep 2.5
key W -- "W again"
key ] -- "the operations: under way first, then the session's history"
sleep 2
key d -- "cancel it…"
key y -- "…ended: Incus calls a cancelled command a failure"
sleep 1.5
key Escape -- "close"
incus start report --project "$P" >/dev/null 2>&1 &
caption "elsewhere" "someone starts an instance…"
wait_screen "1 failed"; sleep 2.5
caption "1 failed" "…which fails: counted in red until you look"; sleep 2.5
key W -- "the failed start"
key Enter -- "and why"
sleep 2
key Escape -- "close"
key Escape -- "close"
caption "W" "operations and warnings"; sleep 4
# -------------------------------------------------------------------------

finish_recording
kill "$JOB" 2>/dev/null || true
render
