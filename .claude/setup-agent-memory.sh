#!/usr/bin/env bash
# Point this checkout's live agent memory at the shared external store.
# Run ONCE per checkout and per new worktree. Idempotent and non-destructive.
#
# Why: live agent memory must be shared across worktrees AND outside git, so it
# survives checkouts (the #636 goal) without conflicting on every pull (the #638
# problem). It lives at a stable external path; each checkout symlinks to it.
set -euo pipefail

target="$HOME/.claude/silt-agent-memory"
link=".claude/agent-memory"

mkdir -p "$target"

# Already linked correctly → done.
if [ -L "$link" ] && [ "$(readlink "$link")" = "$target" ]; then
  echo "ok: $link already -> $target"
  exit 0
fi

# Refuse to clobber a real directory or a wrong symlink — the caller decides.
if [ -e "$link" ] || [ -L "$link" ]; then
  echo "refusing: $link exists and is not the expected symlink."
  echo "  If it holds memory you want, move it into $target first, then remove $link and re-run."
  exit 1
fi

ln -s "$target" "$link"
echo "linked: $link -> $target"
