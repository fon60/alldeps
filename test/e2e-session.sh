#!/usr/bin/env bash
# End-to-end session test (task 3.3): launch npmitude in a pty against a
# throwaway nvm layout with TWO node prefixes, search for a package, pick both
# destinations from the install-target popup, confirm the plan, and verify the
# two-destination plan is applied in one run (one manager invocation per
# destination) and the package lands on disk in both prefixes.
# Runnable from a clean checkout: builds the binary if missing.
set -euo pipefail

cd "$(dirname "$0")/.."

[ -x ./npmitude ] || ./build.sh

BASE=$(mktemp -d "${TMPDIR:-/tmp}/npmitude-e2e.XXXXXX")
PREFIX_A="$BASE/versions/node/v18.0.0"
PREFIX_B="$BASE/versions/node/v16.0.0"
trap 'rm -rf "$BASE"' EXIT

# Build two throwaway nvm-layout prefixes from the running node + its npm.
NODE_BIN="$(command -v node)"
NODE_DIR="$(dirname "$(dirname "$NODE_BIN")")"
for P in "$PREFIX_A" "$PREFIX_B"; do
  mkdir -p "$P/bin" "$P/lib/node_modules"
  cp "$NODE_BIN" "$P/bin/node"
  cp -a "$NODE_DIR/lib/node_modules/npm" "$P/lib/node_modules/"
done

PKG="pad-left"
OUT="$BASE/session.txt"

# The throwaway prefixes are put first on PATH so PREFIX_A resolves as the
# active npm prefix and opens by default (no picker navigation needed).
# Keys are sent through a SIGPIPE-tolerant helper: once the app quits, later
# writes to the closed pty must not fail the pipeline. An external printf is
# used on purpose — a builtin killed by SIGPIPE would take the subshell down.
p() { /usr/bin/printf "$@" 2>/dev/null || true; }
{
  sleep 12
  p '/'
  sleep 1
  p 'pad-left'
  sleep 5
  p '\r'
  sleep 8
  p '+'
  sleep 2
  # Install-target popup: include the first destination with +, move down,
  # include the second one as well, then confirm.
  p '+'
  sleep 0.5
  p 'j'
  sleep 0.5
  p '+'
  sleep 0.5
  p '\r'
  sleep 2
  p 'g'
  sleep 3
  p 'g'
  # Apply run: poll the completion prompt, dismiss it (back on the plan tab),
  # close the plan tab, then quit from the list (confirming if marks remain).
  # Keys sent while a batch is still running are ignored by the apply screen.
  for i in 1 2 3 4 5 6; do
    sleep 15
    p '\r'
    sleep 1
    p 'q'
    sleep 1
    p 'q'
    sleep 1
    p 'y'
    sleep 1
  done
} | env PATH="$PREFIX_A/bin:$PATH" NVM_DIR="$BASE" \
  script -qec "stty cols 80 rows 24; ./npmitude" /dev/null > "$OUT" 2>&1

sed 's/\x1b\[[0-9;?]*[a-zA-Z]//g' "$OUT" | tr -d '\r' > "$BASE/clean.txt"

fail() { echo "E2E FAIL: $1"; echo "--- session tail ---"; grep -vE '^ *$' "$BASE/clean.txt" | tail -30; exit 1; }

grep -q 'choose destination(s)' "$BASE/clean.txt" || fail "install-target popup was not shown"
grep -q 'install mark updated on 2 destination(s)' "$BASE/clean.txt" || fail "popup did not record two install marks"
grep -q '^Plan' "$BASE/clean.txt" || fail "plan preview was not shown"
grep -Eq 'pad-left@[0-9]' "$BASE/clean.txt" || fail "plan did not list pad-left with a version"
grep -q 'Apply finished' "$BASE/clean.txt" || fail "apply run did not complete"

# Both destinations must appear in the apply log as their own batch, proving
# one run spanned both groups (each through its destination's manager).
grep -Eq "pad-left@[0-9][^ ]* @ .*/v18\.0\.0" "$BASE/clean.txt" || fail "apply log missing the v18 group batch"
grep -Eq "pad-left@[0-9][^ ]* @ .*/v16\.0\.0" "$BASE/clean.txt" || fail "apply log missing the v16 group batch"

# Verify on disk: the package must be installed in BOTH prefixes at the exact
# version the plan applied.
WANT=$(grep -Eo 'pad-left@[0-9][^ ]*' "$BASE/clean.txt" | head -1 | cut -d@ -f2)
for P in "$PREFIX_A" "$PREFIX_B"; do
  LS=$("$P/bin/node" "$P/lib/node_modules/npm/bin/npm-cli.js" ls -g --depth=0 2>&1) || true
  echo "$LS" | grep -q "pad-left@$WANT" || fail "pad-left@$WANT is not installed in $P: $LS"
done

echo "E2E PASS: two-destination plan applied in one run (search -> popup -> mark x2 -> plan -> apply)"
