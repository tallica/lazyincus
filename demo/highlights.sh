#!/usr/bin/env bash
# Cuts demo.sh's recording down to a short gif for the README. Each clip starts at
# a caption rather than a timestamp, so a re-recording with different daemon waits
# still cuts in the same places.
#   brew install agg ffmpeg jq
#   demo/highlights.sh [demo/demo.cast] [docs/highlights.gif]
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
CAST=${1:-$HERE/demo.cast}
GIF=${2:-$HERE/../docs/highlights.gif}
FONT=${FONT:-JetBrainsMono Nerd Font Mono}
SPEED=${SPEED:-1.2}

# caption|seconds — each caption is looked for after the one before it
CLIPS=(
  "Getting around|9.7"
  "focus the main panel|5.6"
  "Top — live processes|1.9"
  "stop…|8.1"
  "exec a shell|8.4"
  "instances using this image|3.8"
  "stop the cache service…|6.9"
  "web: two replicas|6.5"
  "fold away|7"
  "add a stack…|13"
  "stop… the prompt names the remote|2.5"
  "switch the whole screen|9"
)

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# every output event as "time<TAB>text", escape sequences stripped, to find the captions in
tail -n +2 "$CAST" | jq -r 'select(.[1] == "o")
  | "\(.[0])\t\(.[2] | gsub("\u001b\\[[0-9;?]*[A-Za-z]"; "") | gsub("[\t\r\n]"; " "))"' >"$TMP/text"

filter="" inputs="" n=0 from=0
for clip in "${CLIPS[@]}"; do
  label=${clip%|*} len=${clip#*|}
  at=$(awk -F'\t' -v l="$label" -v from="$from" '$1 > from && index($2, l) { print $1; exit }' "$TMP/text")
  [[ $at ]] || { echo "caption not found: $label" >&2; exit 1; }
  start=$(awk -v t="$at" 'BEGIN { s = t - 0.2; print (s < 0 ? 0 : s) }')
  filter+="[0:v]fps=15,trim=$start:$(awk -v s="$start" -v l="$len" 'BEGIN { print s + l }'),setpts=PTS-STARTPTS[c$n];"
  inputs+="[c$n]" from=$at n=$((n + 1))
done
# a held last frame gives the loop a beat before it starts over
filter+="${inputs}concat=n=$n:v=1:a=0,setpts=PTS/$SPEED,tpad=stop_mode=clone:stop_duration=2,fps=12,"
filter+="split[a][b];[a]palettegen=max_colors=256:stats_mode=full[p];[b][p]paletteuse=dither=none"

# no idle-time limit: the trims are in the cast's own time
agg --font-family "$FONT" --font-size 24 --idle-time-limit 1000 --fps-cap 15 --last-frame-duration 0 -q "$CAST" "$TMP/full.gif"
ffmpeg -y -loglevel error -i "$TMP/full.gif" -filter_complex "$filter" -loop 0 "$GIF"
echo "wrote $GIF"
