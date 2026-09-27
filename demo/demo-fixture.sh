#!/usr/bin/env bash
# Throwaway fixture for demo.sh:  demo-fixture.sh up | down
# Everything lives in project lzi-demo, the lzi-stack compose project, and network lzidemo0.
set -uo pipefail
exec </dev/null    # incus create/edit read YAML from a non-tty stdin
: "${INCUS_REMOTE:?set INCUS_REMOTE to the demo daemon}"
P=lzi-demo
DIR=$(cd "$(dirname "$0")" && pwd)
i() { incus "$@" --project "$P"; }

up() {
  incus project create "$P" -c features.networks=false
  i profile device add default root disk path=/ pool=default
  i profile device add default eth0 nic network=incusbr0 name=eth0
  i profile create demo-unused
  i profile set demo-unused limits.cpu=1
  incus network create lzidemo0 ipv4.address=10.77.0.1/24 ipv6.address=none
  i launch images:alpine/edge alpha
  i launch images:alpine/edge beta
  i init   images:alpine/edge gamma
  i snapshot create alpha before-upgrade
  i snapshot create alpha nightly
  incus storage volume create default demo-data --project "$P"
  incus storage volume snapshot create default demo-data first --project "$P"
  i publish alpha/nightly --alias demo-published
  (cd "$DIR/lzi-stack" && incus-compose up --detach)
}

down() {
  (cd "$DIR/lzi-stack" && incus-compose down --volumes)
  for c in alpha beta gamma; do i delete -f "$c" 2>/dev/null; done
  incus storage volume delete default demo-data --project "$P" 2>/dev/null
  for f in $(i image list -f csv -c f); do i image delete "$f"; done
  i profile delete demo-unused 2>/dev/null
  incus network delete lzidemo0 2>/dev/null
  i profile device remove default root 2>/dev/null; i profile device remove default eth0 2>/dev/null
  incus project delete "$P"
  incus project delete lzi-stack 2>/dev/null
  true
}

"${1:?up or down}"
