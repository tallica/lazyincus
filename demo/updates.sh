#!/usr/bin/env bash
# Records newer images found and taken: U and G on containers launched by
# hand, then incus-beacon's verdict on a stack's row, its Services column and
# Info tab, and G editing the compose file and recreating the service. Its
# fixture is the lzi-upd stack, its gateway on an older nginx tag beacon
# tracks, and beside it in its project cdn, a paused edge and a snapshotted
# docs on the same tag; built afresh and left behind, `demo/updates.sh down`
# removes it. Needs incus-beacon running on REMOTE_A with events on. How to
# run it is demo/README.md.
set -euo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
export REMOTE_A=${REMOTE_A:-site-a}
ROWS=${ROWS:-34} PAUSE=${PAUSE:-2} TYPE_DELAY=${TYPE_DELAY:-0.15} LEAD=${LEAD:-2.5}
# shellcheck source=demo/lib.sh
source "$(dirname "$0")/lib.sh"
P=lzi-upd
HAND=(cdn edge docs)

compose() { (cd "$HERE/$P" && incus-compose "$@"); }

down() {
  local c f
  for c in "${HAND[@]}"; do incus delete -f "$c" --project "$P" 2>/dev/null || true; done
  compose down --volumes >/dev/null 2>&1 || true
  # down leaves the pulled images behind, and a project goes only once empty.
  for f in $(incus image list -f csv -c f --project "$P" 2>/dev/null); do
    incus image delete "$f" --project "$P" || true
  done
  incus project delete "$P" 2>/dev/null || true
}

# beacon_says <status> <instance>… — until beacon has written it on each
beacon_says() {
  local s c _
  s=$1; shift
  for c in "$@"; do
    for _ in $(seq 120); do
      [[ $(incus config get "$c" user.incus-beacon.status --project "$P" 2>/dev/null) == "$s" ]] && continue 2
      sleep 0.5
    done
    echo "incus-beacon never said $s on $c" >&2; return 1
  done
}

up() {
  local c
  compose up --detach
  # The project's default profile is incus-compose's, with no root disk or NIC.
  for c in "${HAND[@]}"; do
    incus launch docker:library/nginx:1.27-alpine "$c" --project "$P" \
      -s default -n incusbr0 -c user.incus-beacon.track=semver
  done
  incus pause edge --project "$P"
  incus snapshot create docs before --project "$P"
  beacon_says update-available gateway-1 "${HAND[@]}"
}

if [[ ${1:-} == down ]]; then down; exit; fi
[[ ${SKIP_FIXTURE:-} ]] || { down >/dev/null 2>&1; up >/dev/null; }

cp -R "$HERE/$P" "$DEMO_HOME/"
demo_home "$REMOTE_A"
printf 'stacks:\n  - %s:%s/%s\n' "$REMOTE_A" "$DEMO_HOME" "$P" >"$CFG/state.yml"
start_app
caption "lazyincus" "newer images, found and taken"
# Scoped to the fixture's project before the recording starts.
wait_screen "Incus v"
steps=$(project_steps "$P")
t send-keys -t app P; sleep 0.5
for ((n = 0; n < steps; n++)); do t send-keys -t app j; sleep 0.1; done
t send-keys -t app Enter
wait_screen "$REMOTE_A/$P)"
t send-keys -t app 3    # Instances
wait_screen "↑1.31"
start_recording

# --- storyboard ----------------------------------------------------------
# Hand-launched first: a rebuild from the image incus-compose cached would
# leave beacon no tag to read on 0.17.0.
chapter "Containers launched by hand"
caption "↑1.31" "nginx:1.27-alpine, no compose file; incus-beacon, on the server, found 1.31"; sleep 3.5
key U -- "U: recreate one from its image"
caption "U" "as it is, or after pulling the latest build of its tag"; sleep 2.5
key Escape -- "not just this one"
key G -- "G: every OCI container no stack owns with a newer build or tag, from → to"
caption "docs" "left out: Incus won't recreate an instance with snapshots"; sleep 3
key Enter -- "the root disk is replaced; config, devices and volumes stay"
key Enter -- "one after another, each from a fresh pull"
beacon_says current cdn edge
wait_state "$P" edge FROZEN
sleep 2
caption "frozen" "edge paused again after, and beacon has nothing newer for either"; sleep 3.5
chapter "A compose stack"
key 1 -- "Stacks: ↑ after the status, something newer in the stack"
key 2 -- "Services: which one, and what"
caption "↑1.31" "a newer tag of the image gateway runs, without the -alpine both share"; sleep 3
caption "Newer tag" "the Info tab: the tag, and since when"; sleep 3
key G -- "G: the stack's services with a newer build or tag"
key Enter -- "the compose file is edited first, and says so"
key Enter -- "the new tag into compose.yaml, then pull and recreate"
back_from_shell
beacon_says current gateway-1
caption "1.31-alpine" "on the new tag, and beacon has nothing newer: the ↑ is gone"; sleep 4
# -------------------------------------------------------------------------

finish_recording
render
