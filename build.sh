#!/usr/bin/env bash
# Build the npmitude static binary into ./npmitude (repo root).
set -euo pipefail

cd "$(dirname "$0")"

out="./npmitude"
echo "building ${out} ..."
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "${out}" .
echo "built ${out}"
