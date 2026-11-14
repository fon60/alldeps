# add-lint-gate

## Why

The project has no code-quality gate: nothing enforces formatting or static analysis before a build, the two locally installed analyzers (golangci-lint v1.60.3, staticcheck 2024.1.1) are broken against the Go 1.27 toolchain and cannot run at all, and there is no way to check dependencies for known CVEs before cutting a release.

## What Changes

- New `lint.sh`: single local command running gofmt check → `go vet ./...` → golangci-lint v2 (pinned v2.15.0 via `go run`, no external service).
- **BREAKING** (build flow): `build.sh` runs the lint gate before compiling; any finding means no binary is produced.
- New `.golangci.yml` (v2 format): default linters + gofmt, with the idiomatic deferred-Close errcheck exclusions.
- Upgrade installed golangci-lint v1.60.3 → v2.15.0 and staticcheck 2024.1.1 → current pinned release; both are currently unusable on Go 1.27. Standalone staticcheck is NOT added as a gate step — golangci-lint already executes it.
- One-time cleanup so the first gated build is green: gofmt 16 files, fix ~7 real findings, delete dead code `(*Manager).lockPath` (internal/lock/lock.go:150).
- New `vuln.sh`: govulncheck (pinned v1.9.0) reachability-based CVE scan of the module graph and stdlib; mandatory pre-release step in `docs/release.md`.
- AGENTS.md Setup Commands gains Lint and Vuln entries.

## Capabilities

### New Capabilities

- `static-analysis`: local-only static analysis and vulnerability scanning — build gate behavior, pinned tooling versions, and the pre-release vuln scan requirement.

### Modified Capabilities

(none)

## Impact

- Code: one-time fixes under `internal/` (gofmt on 16 files, ~7 findings incl. dead code in internal/lock).
- New files: `.golangci.yml`, `lint.sh`, `vuln.sh`.
- Modified: `build.sh`, `AGENTS.md`, `docs/release.md`.
- Dependencies: go.mod/go.sum untouched — tools are pinned via `go run pkg@version` / `go install pkg@version`.
- Local toolchain: golangci-lint and staticcheck binaries in ~/go/bin upgraded.
- Non-goals: SonarQube (deferred), CI integration (no CI exists for this repo).
