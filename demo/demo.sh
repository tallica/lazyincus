#!/usr/bin/env bash
# Records a paced, captioned walkthrough of every lazyincus key, destructive ones
# included, against the throwaway fixture from demo-fixture.sh, on two remotes.
# How to run it, and what it needs, is demo/README.md.
set -euo pipefail
export REMOTE_A=${REMOTE_A:-site-a} REMOTE_B=${REMOTE_B:-site-b}
# shellcheck source=demo/lib.sh
source "$(dirname "$0")/lib.sh"
DEMO_PROJECT=lzi-demo
STACK_PROJECT=lzi-stack

[[ ${SKIP_FIXTURE:-} ]] || { "$HERE/demo-fixture.sh" down >/dev/null 2>&1; "$HERE/demo-fixture.sh" up >/dev/null; }

cp -R "$HERE/lzi-stack" "$HERE/lzi-edge" "$DEMO_HOME/"
demo_home "$REMOTE_A" "$REMOTE_B"

# Both stacks pinned to their remote, saved as `a` would: no local stack, as
# lazyincus starts outside one, so a switch moves the * and nothing else.
printf 'stacks:\n  - %s:%s/lzi-stack\n' "$REMOTE_A" "$DEMO_HOME" >"$CFG/state.yml"

project_steps=$(project_steps "$DEMO_PROJECT")
# The Remotes menu lists the instance remotes by name, from its first row.
remote_steps=$(INCUS_CONF=$DEMO_HOME/incus incus remote list -f csv -c npP |
  awk -F, '$2 == "incus" && $3 == "NO" { sub(/ \(current\)/, "", $1); print $1 }' |
  LC_ALL=C sort | grep -nx "$REMOTE_B" | cut -d: -f1)
remote_steps=$((remote_steps - 1))
# Stacks lists each remote's together, by name: from lzi-edge to lzi-stack.
to_stack_a=j; [[ $REMOTE_A < $REMOTE_B ]] && to_stack_a=k

start_app
caption "lazyincus" "a terminal UI for Incus"
start_recording

# --- storyboard ----------------------------------------------------------
chapter "Getting around"
key 2 -- "Services: the stack's, from compose.yaml"
key j -- "move down"
key j -- "a replicated service lists its instances"
key k k -- "move up"
key Tab -- "next side panel"
key Tab -- "next side panel"
key Tab -- "next side panel"
key BTab -- "previous side panel"
key BTab -- "…Instances, a long list"
key '=' -- "expand the focused panel, collapse the rest"
key BTab -- "the room follows focus"
key Tab -- "…back"
key '=' -- "split the column evenly again"

chapter "Projects"
key P -- "switch Incus project"
repeat j "$project_steps" "…down to $DEMO_PROJECT"
key Enter -- "scope everything to $DEMO_PROJECT"

chapter "The main panel"
key 3 -- "Instances"
key Enter -- "focus the main panel"
key ] -- "Logs"
key ] -- "Config"
key ] -- "Env"
key ] -- "Top — live processes"
key [ -- "previous tab"
key m -- "straight to Logs"
key Escape -- "back to the list"

chapter "Instance lifecycle"
key j -- "beta"
key s -- "stop…"
key y -- "…confirmed"; wait_state $DEMO_PROJECT beta STOPPED
key S -- "start"; wait_state $DEMO_PROJECT beta RUNNING
key p -- "pause (freeze)"; wait_state $DEMO_PROJECT beta FROZEN
key p -- "unpause"; wait_state $DEMO_PROJECT beta RUNNING
key r -- "restart"; wait_state $DEMO_PROJECT beta RUNNING
key j -- "gamma, stopped"
key S -- "start it"; wait_state $DEMO_PROJECT gamma RUNNING
key d -- "delete…"
key y -- "…confirmed"
key y -- "it's running — stop it and delete anyway"; wait_state $DEMO_PROJECT gamma gone

chapter "Snapshots"
key k k -- "alpha"
key n -- "new snapshot"
type_text "pre-demo" "name it"
key Tab -- "to the options"
key Right Right -- "expires in 7 days"
key Enter -- "create"
key r -- "restore alpha to it…"
key y -- "…confirmed"
key d -- "delete the snapshot…"
key y -- "…confirmed"
key e -- "every instance's snapshots"
key e -- "back to alpha's"

chapter "Into the instance"
key 3 -- "Instances"
key E -- "exec a shell"
type_text "hostname; cat /etc/alpine-release" "run something"
key Enter -- "run it"
type_text "exit" "leave the shell"
key Enter -- "exit"
back_from_shell
key a -- "attach to the console"
key Enter -- "wake the login prompt"
key C-a q -- "detach"
back_from_shell
key c -- "edit its config in \$EDITOR"
key j j j j j j j j -- "incus config edit"
type_text ":q!" "leave unchanged"
key Enter -- "quit the editor"
back_from_shell
key y -- "copy…"
key j -- "the IPv4 address"
key Enter -- "copied to the clipboard"

chapter "Resources"
key 5 -- "Images"
key u -- "instances using this image"
key Escape -- "back"
key D -- "prune images…"
key j -- "every unused one"
key Enter -- "names each"
key y -- "…confirmed"
key 5 -- "again: Volumes"
pick demo-data
key d -- "delete the custom volume…"
key y -- "…confirmed"
key Escape -- "clear the filter"
key 5 -- "Networks"
key e -- "the host's own interfaces too"
key e -- "managed networks only"
pick lzidemo0
key d -- "delete the network…"
key y -- "…confirmed"
key Escape -- "clear the filter"
key 5 -- "Profiles"
pick demo-unused
key d -- "delete a profile nothing uses…"
key y -- "…confirmed"
key Escape -- "clear the filter"

chapter "A compose stack"
key 1 -- "Stacks: compose projects, each saved on its remote"
key Enter -- "its Info: endpoints, usage, drift"
key ] -- "every service's logs"
key ] -- "the whole compose config"
key [ [ -- "back to Info"
key Escape -- "back to the list"
key 2 -- "its services"
key s -- "stop the cache service…"
key y -- "…confirmed"; back_from_shell
key S -- "start it"; back_from_shell
key j -- "web: two replicas"
key p -- "pause the service"; back_from_shell
key p -- "unpause"; back_from_shell
key r -- "restart"; back_from_shell
key j -- "one replica"
key s -- "stop just this replica…"
key y -- "…confirmed"; wait_state $STACK_PROJECT web-1 STOPPED
key k -- "the service"
key u -- "bring it up again"; back_from_shell
key g -- "pull its image"; back_from_shell
key U -- "pull and recreate…"
key y -- "…confirmed"; back_from_shell
key d -- "bring it down…"
key Enter -- "down…"
key y -- "…confirmed"; back_from_shell
key 1 -- "Stacks: the same verbs, for the whole stack"
key u -- "bring it all up"; back_from_shell

chapter "Two remotes"
key R -- "the remotes menu"
repeat j "$remote_steps" "…down to $REMOTE_B"
key Enter -- "no stack on $REMOTE_B: Stacks and Services fold away"
sleep 3
key 1 -- "focus Stacks and Instances folds instead"
key a -- "add a stack…"
type_text "$REMOTE_B:~/lzi-edge" "…pinned to a remote, the way incus names it"
key Enter -- "a remote column, * on the one the screen is on"
sleep 3
key Enter -- "every Info tab starts with where it lives"
key Escape -- "back to the list"
key 2 -- "its services"
key Enter -- "remote, project, stack, service"
key Escape -- "back to the list"
key 1 -- "Stacks"
key "$to_stack_a" -- "lzi-stack, still on $REMOTE_A"
key 2 -- "its services, read from $REMOTE_A"
key s -- "stop… the prompt names the remote"
key y -- "…confirmed"; back_from_shell
key S -- "start it"; back_from_shell
key 1 -- "Stacks"
key Space -- "switch the whole screen back to $REMOTE_A"
sleep 3
key 3 -- "$REMOTE_A's instances"
key 1 -- "the list stays put; * follows"
sleep 2

chapter "Make it yours"
key O -- "edit lazyincus's config"
type_text ":%s/rounded/double/" "change the border"
key Enter -- "…"
type_text ":wq" "save"
key Enter -- "applied without a restart"
back_from_shell
key + -- "half-screen mode"
key + -- "full-screen mode"
key _ -- "back"
key _ -- "normal"
key '?' -- "every keybinding, for where you are"
sleep 2
key Escape -- "close"
caption q "quit"; sleep 2      # stop recording here: quitting ends on tmux's [exited] screen
# -------------------------------------------------------------------------

finish_recording
render
"$HERE/highlights.sh" "$OUT.cast"
