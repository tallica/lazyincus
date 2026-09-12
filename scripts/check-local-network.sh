#!/bin/sh
# Is macOS Local Network Privacy blocking us from an Incus daemon?
#
# macOS 15+ blocks ad-hoc signed binaries — which is every Go binary built or
# installed outside the App Store, lazyincus and the incus CLI included — from
# reaching other hosts on the local network until the app that launched them
# holds Local Network permission. The kernel reports that denial as
# EHOSTUNREACH, which Go prints as "connect: no route to host", so it reads as
# a firewall or routing fault on the far end.
#
# This dials the same host:port twice, once from an Apple-signed binary (exempt)
# and once from a freshly built Go binary (not exempt). A split verdict is the
# signature.
#
# Usage: scripts/check-local-network.sh <host> [port]

set -eu

host=${1:-}
port=${2:-8443}

if [ -z "$host" ]; then
	echo "usage: $0 <host> [port]" >&2
	exit 2
fi

if [ "$(uname -s)" != "Darwin" ]; then
	echo "Local Network Privacy is macOS-only — nothing to check on $(uname -s)."
	exit 0
fi

printf 'apple-signed (/usr/bin/nc): '
if /usr/bin/nc -z -G 5 "$host" "$port" >/dev/null 2>&1; then
	apple=ok
	echo OK
else
	apple=fail
	echo FAIL
fi

dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
cat > "$dir/main.go" <<'EOF'
package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	conn, err := net.DialTimeout("tcp", os.Args[1], 5*time.Second)
	if err != nil {
		fmt.Println("FAIL", err)
		return
	}
	conn.Close()
	fmt.Println("OK")
}
EOF

printf 'ad-hoc signed (go):         '
if ! command -v go >/dev/null 2>&1; then
	echo "SKIP (no go on PATH)"
	exit 0
fi
out=$( cd "$dir" && go mod init lnpcheck >/dev/null 2>&1 && go run . "$host:$port" 2>&1 ) || true
echo "$out"
case "$out" in
OK*) adhoc=ok ;;
*) adhoc=fail ;;
esac

echo
if [ "$apple" = ok ] && [ "$adhoc" = fail ]; then
	cat <<EOF
Local Network Privacy is blocking you.

Grant the terminal you launch lazyincus from under System Settings ->
Privacy & Security -> Local Network, then fully quit and relaunch it. The
permission is attributed to the launching app rather than to the binary, and
it is cached per process from launch, so toggling it under a running terminal
changes nothing until that terminal restarts.
EOF
elif [ "$apple" = ok ] && [ "$adhoc" = ok ]; then
	echo "Not Local Network Privacy — $host:$port is reachable either way."
elif [ "$apple" = fail ] && [ "$adhoc" = fail ]; then
	echo "$host:$port is unreachable for both — the daemon, the network or a"
	echo "firewall on the far end, not a macOS permission."
else
	echo "Unexpected: the ad-hoc binary connected where the Apple-signed one did not."
fi
