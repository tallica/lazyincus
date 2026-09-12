# Remotes

lazyincus connects to whichever remote the `incus` CLI treats as its default.
It reads Incus's own client configuration (`~/.config/incus/config.yml`, or
`$INCUS_CONF`) rather than looking for a socket itself, so any remote the CLI
can reach — a local unix socket, a daemon in a VM, a server across the network
— works without lazyincus knowing anything about it.

There is no `--remote` flag. Pick a remote with the environment variable
instead:

```sh
INCUS_REMOTE=myserver lazyincus
```

The footer shows the remote you ended up on, so you can tell at a glance
which daemon you are looking at.

`incus remote switch myserver` works too, and persists. Prefer `INCUS_REMOTE`
when you have more than one daemon: `a` (console) and `E` (exec) shell out to
the `incus` CLI with a bare instance name, and the child process inherits the
variable. Switching the default in one place and not the other would send
those two commands to a different daemon than the panels are showing.

## Adding a remote over TLS

On the server, publish the API and mint a one-time join token:

```sh
incus config set core.https_address=:8443
incus config trust add lazyincus
```

`incus config trust add` prints a token that is good for one use. Trust
passwords were removed in newer Incus releases; tokens replaced them.

On the client:

```sh
incus remote add myserver https://<host>:8443 --token <token>
incus list myserver:
INCUS_REMOTE=myserver lazyincus
```

`incus list` is worth running first — it separates "the remote is misconfigured"
from "lazyincus is misbehaving", and its errors are more direct.

## Reaching a daemon over SSH

Where the API port is not reachable — no TLS listener, a firewall in between,
or a client-side restriction like the macOS one below — forward it over SSH
and point the remote at loopback:

```sh
ssh -N -L 8443:127.0.0.1:8443 user@<host> &
incus remote add myserver https://127.0.0.1:8443 --token <token>
```

Incus pins the server's certificate by fingerprint rather than by hostname, so
addressing it as `127.0.0.1` does not upset verification.

Forwarding the unix socket instead skips TLS and the token entirely:

```sh
ssh -N -L /tmp/incus-myserver.sock:/var/lib/incus/unix.socket user@<host> &
incus remote add myserver unix:///tmp/incus-myserver.sock
```

Check the socket path on the server first (`ss -lxp | grep incus`) — it varies
by distribution. Delete the local socket file between attempts, since SSH will
not bind over one that already exists.

Either form needs `AllowTcpForwarding yes` in the server's
`/etc/ssh/sshd_config` (`AllowStreamLocalForwarding yes` as well, for the unix
socket variant). Without it SSH reports `administratively prohibited` and the
connection fails with something unrelated-looking, such as `EOF` or
`connection reset by peer`. If the config has a `Match` block covering the user
you log in as, the directive has to go inside that block to take effect. Run
`sshd -t` before restarting sshd on a machine you are connected to.

The tunnel has to be up before lazyincus starts and dies with the SSH session,
so it is a fallback rather than a default. `autossh`, or a systemd user unit or
launchd agent, makes it durable if you end up relying on it.

## Running the daemon in a local VM

A VM on the host — UTM or plain QEMU on macOS, or any hypervisor giving the
guest an address on a host-only network — is a convenient way to get a real
incusd on a machine that cannot run one natively. Three things about that setup
produce errors that point at the wrong component.

### macOS blocks the connection and blames the network

macOS 15 and later require an application to hold Local Network permission
before it can reach other hosts on the local network. The permission is
attributed to the **application that launched the process**, not to the binary
itself, and Apple's own command-line tools are exempt from the check entirely.

So `curl` and `nc` reach the VM while `incus`, `lazyincus`, and anything else
built with Go do not. The kernel reports the denial as `EHOSTUNREACH`, which
Go renders as:

```
connect: no route to host
```

Routing and ARP look perfect, `ping` and `ssh` work, and the error points
firmly at the VM. Nothing on the VM is wrong.

```sh
scripts/check-local-network.sh <host> 8443
```

dials the same address from an Apple-signed binary and from a freshly built Go
binary. If the first succeeds and the second fails, this is what you are
looking at.

The fix is to grant your terminal Local Network access under **System Settings
→ Privacy & Security → Local Network**, and then **fully quit and relaunch
it** — ⌘Q, not a new window. The decision is cached per process from the moment
it launches, so flipping the toggle under a running terminal changes nothing
and makes the fix look ineffective. Confirm the restart actually happened
(`ps -eo pid,lstart,comm | grep -i <your terminal>`) before concluding it did
not work.

Two consequences worth knowing. Launching lazyincus from a different
application means that application needs its own grant. And a launchd agent has
no application to attribute the permission to, so scheduled or headless runs
need the SSH tunnel above instead.

If the permission was previously denied, the prompt will not reappear on its
own; `tccutil reset LocalNetwork <bundle-id>` clears the decision so the next
attempt asks again.

### A drifting guest clock rejects your certificate

```
Error: The provided certificate isn't valid yet
```

This comes from the server, about *your client* certificate. Incus generates
the client certificate with a validity window starting at the moment it is
created; if the guest's clock is behind the host's, your certificate is dated
in the future as far as the server is concerned, and it refuses it.

Compare the two clocks and sync the guest:

```sh
date -u                        # on the host
ssh user@<host> date -u        # on the guest
```

A VM guest generally has no time synchronization with the host, so install a
proper NTP client rather than setting the clock by hand — the skew returns on
every boot and suspend, and the error gives no hint that time is involved. On
Alpine:

```sh
apk add chrony
rc-update add chronyd default
rc-service chronyd start
```

Nothing needs regenerating afterwards. The existing client certificate becomes
valid the moment the guest's clock passes the certificate's start time.

### The guest has to be reachable at all

Give the VM a networking mode that puts it on a host-visible address — UTM's
"Shared Network" does this, handing out addresses on a private subnet — rather
than a NAT mode where the host cannot address the guest directly. Confirm with
`ssh` before touching Incus configuration, since every diagnostic below that
point assumes the host can reach the guest.

## Troubleshooting

| Error | Cause |
| --- | --- |
| `connect: no route to host`, while `curl`/`ssh` to the same host work | macOS Local Network Privacy — grant the terminal and relaunch it |
| `The provided certificate isn't valid yet` | The server's clock is behind the client's; sync the server |
| `administratively prohibited` | `AllowTcpForwarding no` on the SSH server |
| `channel N: open failed: connect failed` | The forwarded unix socket path does not exist on the server |
| `not authorized` on `remote add` | The token was already used or has expired; mint a new one |
| lazyincus shows a different daemon than `incus` does | `INCUS_REMOTE` and the CLI's default remote disagree |
