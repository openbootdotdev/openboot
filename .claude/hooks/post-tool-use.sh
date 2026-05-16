#!/bin/bash
#
# PostToolUse hook — fires after every Edit / Write / MultiEdit.
#
# For .go file edits: runs `go vet` on the touched package so Claude
# gets immediate feedback if the edit didn't compile / vet cleanly.
#
# Exit code semantics (per Claude Code hook contract):
#   0  → success, stdout shown in transcript (Claude sees it on subsequent turns)
#   2  → blocks: stderr is fed to Claude as feedback to self-correct
#
# We choose exit 2 on vet failure so the agent can fix without waiting
# for Stop. Exit 0 on success — silent path, no noise.

set -uo pipefail

project_dir="${CLAUDE_PROJECT_DIR:-$(pwd)}"
cd "$project_dir"

# Read tool input JSON from stdin and extract the file_path.
input=$(cat)
file_path=$(printf '%s' "$input" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" 2>/dev/null || true)

# No path / non-Go file → exit silently.
[ -z "$file_path" ] && exit 0
case "$file_path" in
  *.go) ;;
  *) exit 0 ;;
esac

# Normalise to a path relative to the project root. Anything outside the
# project (e.g. an edit to ~/.claude/...) is ignored.
case "$file_path" in
  "$project_dir"/*) rel_path="${file_path#"$project_dir"/}" ;;
  /*) exit 0 ;;
  *) rel_path="$file_path" ;;
esac

pkg_dir=$(dirname "$rel_path")
[ -d "$project_dir/$pkg_dir" ] || exit 0

# Run go vet on just the touched package — fast (<1s warm).
if ! out=$(go vet "./$pkg_dir/..." 2>&1); then
  printf '[openboot post-tool-use] go vet failed for ./%s:\n%s\n' "$pkg_dir" "$out" >&2
  exit 2
fi

exit 0
