#!/usr/bin/env bash
# End-to-end session test (task 7.3): launch npmitude in a pty against a
# throwaway nvm-layout prefix, search for a package, mark it for install,
# confirm the plan, and verify the package is actually installed on disk.
# Runnable from a clean checkout: builds the binary if missing.
set -euo pipefail

cd "$(dirname "$0")/.."

[ -x ./npmitude ] || ./build.sh

BASE=$(mktemp -d "${TMPDIR:-/tmp}/npmitude-e2e.XXXXXX")
PREFIX="$BASE/versions/node/v18.0.0"
trap 'rm -rf "$BASE"' EXIT

# Build a throwaway nvm-layout prefix from the running node + its npm.
NODE_BIN="$(command -v node)"
NODE_DIR="$(dirname "$(dirname "$NODE_BIN")")"
mkdir -p "$PREFIX/bin" "$PREFIX/lib/node_modules"
cp "$NODE_BIN" "$PREFIX/bin/node"
cp -a "$NODE_DIR/lib/node_modules/npm" "$PREFIX/lib/node_modules/"

PKG="pad-left"
OUT="$BASE/session.txt"

# The throwaway prefix is put first on PATH so it resolves as the active npm
# prefix and opens by default (no picker navigation needed).
{
  sleep 8
  printf '/'
  sleep 0.5
  printf 'pad-left'
  sleep 4
  printf '\r'
  sleep 6
  printf 'f'
  sleep 0.5
  printf '~n ^pad-left$'
  sleep 2
  printf '\r'
  sleep 1
  printf '+'
  sleep 1
  printf 'g'
  sleep 3
  printf 'g'
  sleep 12
  printf '\r'
  sleep 2
  printf 'q'
  sleep 1
} | env PATH="$PREFIX/bin:$PATH" NVM_DIR="$BASE" \
  script -qec "stty cols 80 rows 24; ./npmitude" /dev/null > "$OUT" 2>&1

CLEAN=$(sed 's/\x1b\[[0-9;?]*[a-zA-Z]//g' "$OUT" | tr -d '\r')

fail() { echo "E2E FAIL: $1"; echo "--- session tail ---"; echo "$CLEAN" | grep -vE '^ *$' | tail -30; exit 1; }

echo "$CLEAN" | grep -q 'Plan —' || fail "plan preview was not shown"
echo "$CLEAN" | grep -Eq 'pad-left@[0-9]' || fail "plan did not list pad-left with a version"
echo "$CLEAN" | grep -q 'Apply finished' || fail "apply run did not complete"

# Verify on disk: the package must be installed in the throwaway prefix at the
# exact version the plan applied.
WANT=$(echo "$CLEAN" | grep -Eo 'pad-left@[0-9][^ ]*' | head -1 | cut -d@ -f2)
LS=$("$PREFIX/bin/node" "$PREFIX/lib/node_modules/npm/bin/npm-cli.js" ls -g --depth=0 2>&1) || true
echo "$LS" | grep -q "pad-left@$WANT" || fail "pad-left@$WANT is not installed on disk: $LS"

echo "E2E PASS: pad-left installed via search -> mark -> plan -> apply"
