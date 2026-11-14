#!/usr/bin/env bash
# Static-analysis gate: go vet + golangci-lint (default linters + gofmt).
set -euo pipefail

cd "$(dirname "$0")"

echo "running go vet ..."
go vet ./...

echo "running golangci-lint ..."
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

echo "lint OK"
