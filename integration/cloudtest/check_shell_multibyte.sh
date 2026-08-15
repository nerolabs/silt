#!/usr/bin/env bash
# Regression guard (build-immutable V5) for the P1-run b525b0b-87478 harness bug:
# an UNBRACED shell variable ($var) placed IMMEDIATELY before a multibyte character
# (e.g. →, …) crashes macOS's bash 3.2 under `set -u` — its identifier parser absorbs
# a byte of the multibyte char into the variable name, yielding an unbound name like
# `h1<0xe2>`. This silently killed the C2 no-capture flow's PASS record() at
# scenarios.sh:832 (`($h1→$h2)`), dropping an otherwise-green verdict.
#
# It is macOS-bash-3.2-specific: Linux/bash-5 (where CI runs the flows in-VM) parses it
# fine, so a RUNTIME test can't catch it — this STATIC lint can, on any platform.
# The fix is always to BRACE the variable: `${var}` unambiguously delimits the name.
#
# Runs anywhere (perl regex, no GNU-grep -P dependency). Exit 1 on any offender.
set -u
cd "$(dirname "${BASH_SOURCE[0]}")"

files=(scenarios.sh lib.sh cloudtest.sh)
# Match `$name` (unbraced) immediately followed by a non-ASCII byte. `${name}` is safe
# (the `{` after `$` is not a name-start char), so braced refs are correctly ignored.
hits="$(perl -ne 'print "$ARGV:$.: $_" if /\$[A-Za-z_][A-Za-z0-9_]*[^\x00-\x7f]/' "${files[@]}" 2>/dev/null || true)"

if [ -n "$hits" ]; then
  echo "✗ unbraced \$var adjacent to a multibyte char — crashes bash 3.2 under set -u (P1 b525b0b harness bug). Brace it: \${var}" >&2
  echo "$hits" >&2
  exit 1
fi
echo "✓ no unbraced \$var-before-multibyte adjacencies in the cloudtest shell harness"
