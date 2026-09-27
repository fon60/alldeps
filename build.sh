#!/usr/bin/env bash
# Build the alldeps static binary into ./alldeps (repo root).
set -euo pipefail

cd "$(dirname "$0")"

out="./alldeps"
echo "building ${out} ..."
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "${out}" .
echo "built ${out}"
