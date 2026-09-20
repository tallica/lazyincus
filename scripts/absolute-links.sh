#!/bin/sh
# Rewrite a markdown file's repo-relative links to absolute ones at a tag.
#
# Usage: scripts/absolute-links.sh v0.8.1 < README.md
#
# README.md and CHANGELOG.md link to their neighbours in the repo — docs/,
# BACKLOG.md, the screenshot. Away from a checkout those links have nothing to
# resolve against: on a release page they hang off the repo root and 404, and
# in a release tarball the neighbours aren't there at all. Pinning them to the
# tag points each one at the tree that release shipped with.
set -eu

tag=${1:-}
if [ -z "$tag" ]; then
	echo "usage: $0 <tag>" >&2
	exit 2
fi

repo=https://github.com/tallica/lazyincus
raw=https://raw.githubusercontent.com/tallica/lazyincus

# An embedded image wants the file itself, not the page GitHub wraps it in.
sed -E \
	-e "s|!\[([^]]*)\]\(([A-Za-z0-9._/-][A-Za-z0-9._/-]*)\)|![\1](${raw}/${tag}/\2)|g" \
	-e "s|\]\(([A-Za-z0-9._/-][A-Za-z0-9._/#-]*)\)|](${repo}/blob/${tag}/\1)|g"
