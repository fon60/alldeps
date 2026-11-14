#!/usr/bin/env bash
# Reachability-based vulnerability scan (govulncheck, source mode).
set -euo pipefail

cd "$(dirname "$0")"

go run golang.org/x/vuln/cmd/govulncheck@latest .
