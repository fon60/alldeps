#!/usr/bin/env bash
# End-to-end pty session test (tasks 2.1, 4.1, 4.2): launch npmitude in a
# pty against a throwaway nvm layout with TWO node prefixes that both carry
# pad-left (different versions -> a Node dedupe conflict). Verify:
#   - the conflicted row carries the distinct "!" list marker without blocking
#     navigation (task 2.1),
#   - opening a plan whose op touches the conflict raises the Yes/No gate;
#     [Yes] navigates to the resolver screen (task 4.1),
#   - [No] renders the plan with the conflicting row marked like the list
#     (task 4.2).
# Offline: packages are hand-crafted package directories, no registry needed.
set -euo pipefail

cd "$(dirname "$0")/.."

[ -x ./npmitude ] || ./build.sh

BASE=$(mktemp -d "${TMPDIR:-/tmp}/npmitude-conflict-gate.XXXXXX")
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
  p '-'        # mark removal on the headline destination (A)
  sleep 2
  p 'g'        # open plan -> conflict gate popup
  sleep 3
  p 'n'        # [No] -> plan with the conflicting row marked
  sleep 3
  p '\033'     # ESC byte: back to list (literal "esc" would type e,s,c)
  sleep 1
  p 'g'        # open plan again -> gate again
  sleep 3
  p 'y'        # [Yes] -> resolver screen for pad-left
  sleep 3
  p '\033'     # back to the plan (gate re-evaluated: still conflicted)
  sleep 2
  p 'n'        # marked plan again
  sleep 2
  p '\033'     # list
  sleep 1
  p 'q'
  sleep 1
  p 'y'
  sleep 1
} | env PATH="$PREFIX_A/bin:$PATH" NVM_DIR="$BASE" \
  script -qec "stty cols 80 rows 24; ./npmitude" /dev/null > "$OUT" 2>&1

sed 's/\x1b\[[0-9;?]*[a-zA-Z]//g' "$OUT" | tr -d '\r' > "$BASE/clean.txt"

fail() { echo "E2E FAIL: $1"; echo "--- session tail ---"; grep -vE '^ *$' "$BASE/clean.txt" | tail -30; exit 1; }

# Task 2.1: the conflicted row carries the distinct flag marker (! in the
# flag cell) while the list is otherwise navigable (the session drives keys
# through it without being forced to resolve).
grep -Eq '^i\*!.*pad-left' "$BASE/clean.txt" || fail "list did not mark the redundant pad-left row with the conflict indicator"

# Task 4.1: the gate popup is shown when opening a plan with unresolved
# conflicts, and [Yes] navigates to the resolver screen.
grep -qF 'There are conflicts in the plan; would you like to resolve them?' "$BASE/clean.txt" || fail "the Yes/No conflict gate was not shown on plan open"
grep -qF 'Resolve — pad-left' "$BASE/clean.txt" || fail "[Yes] did not navigate to the resolver screen"
grep -qF 'Copies by destination' "$BASE/clean.txt" || fail "resolver did not show the per-destination copies table"
grep -Eq 'Align to 2\.0\.0' "$BASE/clean.txt" || fail "resolver did not offer the align option"

# Task 4.2: [No] renders the plan with the conflicting row carrying the same
# indicator as the list.
grep -qF '! conflict' "$BASE/clean.txt" || fail "[No] plan does not mark the conflicting row"
grep -Eq 'remove +pad-left' "$BASE/clean.txt" || fail "plan did not list the pending removal"

echo "E2E PASS: conflict marker on list, plan gate [Yes]->resolver / [No]->marked plan (two-prefix dedupe scenario)"
