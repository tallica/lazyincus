#!/usr/bin/env bash
# Records a read-only remote: REMOTE_B shown as production, read-only and
# red, refusing what would change it, then REMOTE_A as staging, where the
# same key works. Its fixture, project lzi-app with instances api, billing and
# worker on both, is built afresh and left behind; `demo/readonly.sh down`
# removes it. How to run it is demo/README.md.
set -euo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
export REMOTE_A=${REMOTE_A:-site-a} REMOTE_B=${REMOTE_B:-site-b}
ROWS=${ROWS:-34} PAUSE=${PAUSE:-2} TYPE_DELAY=${TYPE_DELAY:-0.15} LEAD=${LEAD:-2.5}
# shellcheck source=demo/lib.sh
source "$(dirname "$0")/lib.sh"
P=lzi-app

down_on() {
  local c
  for c in $(INCUS_REMOTE=$1 incus list --project "$P" -f csv -c n 2>/dev/null); do
    INCUS_REMOTE=$1 incus delete -f "$c" --project "$P" || true
  done
  INCUS_REMOTE=$1 incus project delete "$P" 2>/dev/null || true
}

up_on() {
  local c
  export INCUS_REMOTE=$1
  incus project create "$P" -c features.networks=false -c features.images=false
  incus profile device add default root disk path=/ pool=default --project "$P"
  incus profile device add default eth0 nic network=incusbr0 name=eth0 --project "$P"
  for c in api billing worker; do incus launch images:alpine/edge "$c" --project "$P"; done
  # Opening on addresses rather than a column of zeroes.
  until ! incus list --project "$P" -f csv -c 4 | grep -qvF .; do sleep 1; done
  export INCUS_REMOTE=$REMOTE_A
}

if [[ ${1:-} == down ]]; then down_on "$REMOTE_A"; down_on "$REMOTE_B"; exit; fi
[[ ${SKIP_FIXTURE:-} ]] || for r in "$REMOTE_A" "$REMOTE_B"; do down_on "$r" >/dev/null 2>&1; up_on "$r" >/dev/null; done

demo_home production="$REMOTE_B" staging="$REMOTE_A"
cat >>"$CFG/config.yml" <<'EOF'
remotes:
  production:
    readOnly: true
    color: red
  staging:
    color: green
EOF
start_app
caption "lazyincus" "a read-only remote"
# Scoped to the fixture's project before the recording starts.
wait_screen "Incus v"
steps=$(INCUS_REMOTE=$REMOTE_B project_steps "$P")
t send-keys -t app P; sleep 0.5
for ((n = 0; n < steps; n++)); do t send-keys -t app j; sleep 0.1; done
t send-keys -t app Enter
wait_screen "production/$P)"
t send-keys -t app 3    # Instances: P pressed this early can hand the focus back to Stacks
caption "production" "read-only in the config: marked in the footer, and in red"
start_recording

# --- storyboard ----------------------------------------------------------
key j -- "billing"
key s -- "stop it: refused, nothing reaches production"
sleep 1
key Escape -- "close"
key E -- "a shell: refused too, it could change anything inside"
sleep 1
key Escape -- "close"
key C-p -- "the palette"
TYPE_DELAY=0.25 type_text "stop" "marks what would be refused"
sleep 1.5
key Escape -- "close"
key R -- "switch remote"
key j -- "staging, in green"
key Enter -- "not read-only"
key P -- "scope it to $P, as on production"
repeat j "$(project_steps "$P")" "…"
key Enter -- "$P on staging"
key j -- "billing"
key s -- "the same key…"
sleep 1
key y -- "…stops it here"
wait_state "$P" billing STOPPED
key S -- "and starts it again"
wait_state "$P" billing RUNNING
caption "remotes.<name>.readOnly" "or --read-only, for every remote"; sleep 4
# -------------------------------------------------------------------------

finish_recording
render
