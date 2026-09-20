#!/usr/bin/env bash
# End-to-end pty session test for the composer adapter's PHP-version-dependent
# behavior (platform-mismatch conflict). Per project convention, PHP-version
# tests run in docker: npmitude runs INSIDE a pinned php:8.1-cli container
# (the host composer phar runs on the container's PHP), so the solver's
# platform verdict is deterministic against 8.1 instead of the host's PHP.
# Flow: fixture project with one installed dep -> search symfony/console
# (latest requires php > 8.1) -> mark install -> dry-run probe fails on the
# platform mismatch -> conflict marker + plan gate [Yes] -> resolver offers
# the last compatible release line. Requires docker.
set -euo pipefail

cd "$(dirname "$0")/.."

[ -x ./npmitude ] || ./build.sh

BASE=$(mktemp -d "${TMPDIR:-/tmp}/npmitude-composer-docker.XXXXXX")
PROJ="$BASE/proj"
mkdir -p "$PROJ"
printf '{"name":"probe/composer","require":{}}\n' > "$PROJ/composer.json"
# Container runs as root, so fixture files it creates are not removable by the
# host user; cleanup best-effort.
trap 'rm -rf "$BASE" 2>/dev/null || true' EXIT

# Build the fixture inside the pinned-PHP container (creates vendor + installed.json).
docker run --rm -v "$BASE:/w" -v /usr/local/bin/composer:/usr/local/bin/composer:ro \
  -w /w/proj php:8.1-cli bash -c 'command -v git >/dev/null || (apt-get update -qq && apt-get install -y -qq git); composer require symfony/polyfill-mbstring --no-interaction'

OUT="$BASE/session.txt"

# Keys go through a SIGPIPE-tolerant helper (external printf on purpose: a
# builtin killed by SIGPIPE would take the subshell down).
p() { /usr/bin/printf "$@" 2>/dev/null || true; }
{
  sleep 10
  p '/'
  sleep 1
  p 'symfony/console'
  sleep 8
  p '\r'
  sleep 6
  p '+'
  sleep 14          # debounce + dry-run solver probe (php 8.1 vs latest console)
  p 'g'             # open plan -> conflict gate popup
  sleep 4
  p 'y'             # [Yes] -> resolver screen
  sleep 5
  p '\033'          # back to plan/gate
  sleep 2
  p 'n'             # [No] -> marked plan
  sleep 3
  p '\033'
  sleep 1
  p 'q'
  sleep 1
  p 'y'
  sleep 1
} | docker run --rm -i \
    -v "$BASE:/w" -w /w/proj \
    -v "$(pwd)/npmitude":/usr/local/bin/npmitude:ro \
    -v /usr/local/bin/composer:/usr/local/bin/composer:ro \
    php:8.1-cli bash -c 'script -qec "stty cols 80 rows 24; /usr/local/bin/npmitude ." /dev/null' > "$OUT" 2>&1

sed 's/\x1b\[[0-9;?]*[a-zA-Z]//g' "$OUT" | tr -d '\r' > "$BASE/clean.txt"

fail() { echo "E2E FAIL: $1"; echo "--- session tail ---"; grep -vE '^ *$' "$BASE/clean.txt" | tail -40; exit 1; }

grep 'symfony/polyfill-mbstring' "$BASE/clean.txt" | grep -Eq '[0-9]+\.[0-9]+\.[0-9]+' || fail "installed dep not listed with an exact version"
grep -qF 'There are conflicts in the plan; would you like to resolve them?' "$BASE/clean.txt" || fail "conflict gate was not shown on plan open"
grep -qF 'Resolve — symfony/console' "$BASE/clean.txt" || fail "[Yes] did not navigate to the resolver screen"
grep -Eq 'Install symfony/console v[0-9]+\.[0-9]+\.[0-9]+ \(last r' "$BASE/clean.txt" || fail "resolver did not offer the last compatible release line"
grep -qF 'your platform is php 8.1' "$BASE/clean.txt" || fail "conflict consequence does not report the pinned container PHP (8.1.x) — was this run outside php:8.1-cli?"

echo "E2E PASS: composer platform-mismatch conflict under pinned php:8.1 (list -> search -> mark -> gate [Yes] -> resolver)"
