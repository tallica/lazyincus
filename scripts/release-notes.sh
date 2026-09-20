#!/bin/sh
# Print a tag's release notes: its CHANGELOG.md section, then a compare link.
#
# Usage: scripts/release-notes.sh v0.8.1
#
# The release workflow feeds this to `goreleaser release --release-notes`, so a
# tag pushed without a CHANGELOG entry fails the release rather than publishing
# an empty one.
set -eu

tag=${1:-}
if [ -z "$tag" ]; then
	echo "usage: $0 <tag>" >&2
	exit 2
fi

version=${tag#v}
repo=https://github.com/tallica/lazyincus

notes=$(awk -v version="$version" '
	index($0, "## [" version "]") == 1 { found = 1; next }
	found && (index($0, "## ") == 1 || /^\[[^]]+\]: /) { exit }
	found { lines[++count] = $0 }
	END {
		# Trim the blank lines the section boundaries leave at either end.
		first = 1
		while (first <= count && lines[first] == "") first++
		while (count >= first && lines[count] == "") count--
		for (i = first; i <= count; i++) print lines[i]
	}
' CHANGELOG.md)

if [ -z "$notes" ]; then
	echo "$0: no '## [$version]' section in CHANGELOG.md" >&2
	exit 1
fi

printf '%s\n' "$notes" | "$(dirname "$0")/absolute-links.sh" "$tag"

previous=$(git describe --tags --abbrev=0 "$tag^" 2>/dev/null || true)
if [ -n "$previous" ]; then
	printf '\nFull changelog: %s/compare/%s...%s\n' "$repo" "$previous" "$tag"
fi
