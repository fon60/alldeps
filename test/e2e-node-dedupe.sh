#!/usr/bin/env bash
# End-to-end pty session test (tasks 3.1, 6.1, 6.2, 7.1b): the Node
# dedupe/align scenario against a throwaway two-prefix nvm layout. pad-left is
# installed at different versions on both prefixes (redundant copies -> a
# derived dedupe conflict). The session:
#   - opens the resolver from the list with 'r' and verifies the per-destination
#     copies table plus the align option with its size delta (tasks 3.1, 6.1),
#   - picks "Align to 2.0.0" which marks removal of the other copy, then opens
#     the plan WITHOUT the conflict gate and applies it (task 6.2),
#   - verifies on disk that only the aligned copy survives.
# Offline: packages are hand-crafted package directories; apply is a removal,
# which needs no registry.
set -euo pipefail

cd "$(dirname "$0")/.."

[ -x ./npmitude ] || ./build.sh

BASE=$(mktemp -d "${TMPDIR:-/tmp}/npmitude-dedupe.XXXXXX")
PREFIX_A="$BASE/versions/node/v18.0.0"
PREFIX_B="$BASE/versions/node/v16.0.0"
trap 'rm -rf "$BASE"' EXIT

NODE_BIN="$(command -v node)"
NODE_DIR="$(dirname "$(dirname "$NODE_BIN")")"
for P in "$PREFIX_A" "$PREFIX_B"; do
  mkdir -p "$P/bin" "$P/lib/node_modules"
  cp "$NODE_BIN" "$P/bin/node"
  cp -a "$NODE_DIR/lib/node_modules/npm" "$P/lib/node_modules/"
done

# pad-left@1.0.0 on A, pad-left@2.0.0 on B (redundant copies -> conflict).
make_pkg() { # $1=prefix $2=version
  mkdir -p "$1/lib/node_modules/pad-left"
  printf '{"name":"pad-left","version":"%s","description":"left pads"}' "$2" > "$1/lib/node_modules/pad-left/package.json"
  dd if=/dev/zero of="$1/lib/node_modules/pad-left/data.bin" bs=1M count=1 2>/dev/null
}
make_pkg "$PREFIX_A" "1.0.0"
make_pkg "$PREFIX_B" "2.0.0"

OUT="$BASE/session.txt"

# PREFIX_A first on PATH so it resolves as the active npm prefix and opens by
# default. Keys go through a SIGPIPE-tolerant helper (external printf on
# purpose: a builtin killed by SIGPIPE would take the subshell down).
p() { /usr/bin/printf "$@" 2>/dev/null || true; }
{
  sleep 15
  p 'j'        # cursor from npm to pad-left (name order)
  sleep 0.5
  p 'r'        # open the resolver for the conflicted package
  sleep 3
  p '\r'       # apply option under the cursor: "Align to 2.0.0" -> back to list
  sleep 2
  p 'g'        # open plan (no gate: the conflict is resolved)
  sleep 3
  p 'g'        # start the apply run
  sleep 25
  p '\r'
  sleep 2
  p 'q'
  sleep 1
  p 'y'
  sleep 1
} | env PATH="$PREFIX_A/bin:$PATH" NVM_DIR="$BASE" \
  script -qec "stty cols 80 rows 24; ./npmitude" /dev/null > "$OUT" 2>&1

sed 's/\x1b\[[0-9;?]*[a-zA-Z]//g' "$OUT" | tr -d '\r' > "$BASE/clean.txt"

fail() { echo "E2E FAIL: $1"; echo "--- session tail ---"; grep -vE '^ *$' "$BASE/clean.txt" | tail -30; exit 1; }

# Tasks 3.1 / 6.1: the resolver screen lists the per-destination copies with
# versions and sizes, and offers the align option with its size delta.
grep -qF 'Resolve — pad-left' "$BASE/clean.txt" || fail "resolver screen was not shown for pad-left"
grep -qF 'Copies by destination' "$BASE/clean.txt" || fail "resolver did not show the per-destination copies table"
grep -Eq 'Align to 2\.0\.0' "$BASE/clean.txt" || fail "resolver did not offer the align option"
grep -Eq '\(frees [0-9]' "$BASE/clean.txt" || fail "align option did not show a size delta"

# The pick resolved the conflict: opening the plan afterwards must NOT raise
# the gate, and no plan row carries the conflict marker.
grep -qF 'There are conflicts in the plan; would you like to resolve them?' "$BASE/clean.txt" && fail "conflict gate was raised although the conflict had been resolved"
grep -qF '! conflict' "$BASE/clean.txt" && fail "plan row still marked as conflicted after resolving"

# Task 6.2: the align pick produced a removal mark that the apply run executed.
grep -Eq 'remove +pad-left' "$BASE/clean.txt" || fail "plan did not list the aligned-away copy for removal"
grep -q 'Apply finished' "$BASE/clean.txt" || fail "apply run did not complete"

# Verify on disk: only the aligned copy (2.0.0 on B) survives; A's 1.0.0 is gone.
LS_A=$("$PREFIX_A/bin/node" "$PREFIX_A/lib/node_modules/npm/bin/npm-cli.js" ls -g --depth=0 2>&1) || true
echo "$LS_A" | grep -q 'pad-left' && fail "pad-left still installed on the aligned-away prefix A: $LS_A"
LS_B=$("$PREFIX_B/bin/node" "$PREFIX_B/lib/node_modules/npm/bin/npm-cli.js" ls -g --depth=0 2>&1) || true
echo "$LS_B" | grep -q 'pad-left@2.0.0' || fail "pad-left@2.0.0 missing on the kept prefix B: $LS_B"

echo "E2E PASS: resolver align flow -> plan without gate -> apply frees the redundant copy (disk verified)"
