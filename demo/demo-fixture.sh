#!/usr/bin/env bash
# Throwaway fixture for demo.sh:  demo-fixture.sh up | down
# On REMOTE_A: project lzi-demo, the lzi-stack compose project with a few
# snapshots, and network lzidemo0. On REMOTE_B: the lzi-edge compose project.
# On both, a snapshot of incus-compose's ic-healthd.
set -uo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
: "${REMOTE_A:?set REMOTE_A to the first demo daemon}"
: "${REMOTE_B:?set REMOTE_B to the second demo daemon}"
P=lzi-demo
DIR=$(cd "$(dirname "$0")" && pwd)

# a <incus args> — against REMOTE_A, in the demo project
a() { INCUS_REMOTE=$REMOTE_A incus "$@" --project "$P"; }

# compose <remote> <stack dir> <incus-compose args>
compose() { (cd "$DIR/$2" && INCUS_REMOTE=$1 incus-compose "${@:3}"); }

up() {
  export INCUS_REMOTE=$REMOTE_A
  incus project create "$P" -c features.networks=false
  a profile device add default root disk path=/ pool=default
  a profile device add default eth0 nic network=incusbr0 name=eth0
  a profile create demo-unused
  a profile set demo-unused limits.cpu=1
  incus network create lzidemo0 ipv4.address=10.77.0.1/24 ipv6.address=none
  a launch images:alpine/edge alpha
  a launch images:alpine/edge beta
  a init   images:alpine/edge gamma
  a snapshot create alpha before-upgrade
  a snapshot create alpha nightly
  incus storage volume create default demo-data --project "$P"
  incus storage volume snapshot create default demo-data first --project "$P"
  a publish alpha/nightly --alias demo-published
  compose "$REMOTE_A" lzi-stack up --detach
  for c in web-1 web-2 cache-1; do incus snapshot create "$c" before-upgrade --project lzi-stack; done
  incus snapshot create cache-1 nightly --project lzi-stack
  compose "$REMOTE_B" lzi-edge up --detach
  # incus-compose's own health daemon, there once a stack has a healthcheck
  for r in "$REMOTE_A" "$REMOTE_B"; do
    INCUS_REMOTE=$r incus snapshot create ic-healthd before-upgrade --project incus-compose 2>/dev/null
  done
}

# down_on <remote> — everything up made, and what an older fixture left
down_on() {
  export INCUS_REMOTE=$1
  compose "$1" lzi-stack down --volumes
  compose "$1" lzi-edge down --volumes
  incus snapshot delete ic-healthd before-upgrade --project incus-compose 2>/dev/null
  for c in alpha beta gamma; do incus delete -f "$c" --project "$P" 2>/dev/null; done
  incus storage volume delete default demo-data --project "$P" 2>/dev/null
  incus profile delete demo-unused --project "$P" 2>/dev/null
  incus network delete lzidemo0 2>/dev/null
  incus profile device remove default root --project "$P" 2>/dev/null
  incus profile device remove default eth0 --project "$P" 2>/dev/null
  # A project goes only once empty: down leaves a stack's pulled image behind.
  for project in "$P" lzi-stack lzi-edge; do
    for f in $(incus image list -f csv -c f --project "$project" 2>/dev/null); do
      incus image delete "$f" --project "$project"
    done
    incus project delete "$project" 2>/dev/null
  done
  true
}

down() { down_on "$REMOTE_A"; down_on "$REMOTE_B"; }

"${1:?up or down}"
