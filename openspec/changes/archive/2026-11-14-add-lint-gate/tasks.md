# add-lint-gate — Tasks

## 1. Interactive tooling refresh (not part of the gate)

- [x] 1.1 Refresh installed golangci-lint via `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`; verify `golangci-lint version` reports a v2 release and runs without Go 1.27 errors
- [x] 1.2 Refresh installed staticcheck via `go install honnef.co/go/tools/cmd/staticcheck@latest`; verify `staticcheck -version` prints without Go 1.27 export-data errors

## 2. Gate tooling (no version pins anywhere)

- [x] 2.1 Create minimal `.golangci.yml`: `version: "2"` plus gofmt enabled on top of the default set (v2 schema: `formatters.enable: [gofmt]` — gofmt is a formatter, not a linter, in v2), no exclusions; verify `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest config verify` passes
- [x] 2.2 Create `lint.sh` (same shape as build.sh): `go vet ./...` then `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...`; verify it exits non-zero on the current tree and prints the findings
- [x] 2.3 Create `vuln.sh`: `go run golang.org/x/vuln/cmd/govulncheck@latest .` in source mode; verify it runs against the repo and reports "No vulnerabilities found"

## 3. One-time code cleanup (gate must be green before it is wired into build.sh)

- [x] 3.1 Run `gofmt -w` over all Go files; verify `gofmt -l .` (excluding node_modules) prints nothing
- [x] 3.2 Fix the 5 staticcheck findings (ST1005 composer.go:26 and gomod.go:32, SA6003 filter_test.go:39, QF1007 model.go:1161, S1011 model.go:1733); verify `go test ./...` still passes
- [x] 3.3 Delete the unused `(*Manager).lockPath` (internal/lock/lock.go:150); verify `go build ./...` and `go test ./internal/lock/` pass
- [x] 3.4 Handle all errcheck findings without exclusions: explicit error handling for Close/Remove in internal/lock (write-fail cleanup, success-path close at :100, stale-remove at :111, Release at :124 now returns the unlink error, ReleaseAll joins per-file errors), explicit body close with error merge in internal/registry (:79/:153/:226/:372) and internal/adapter/composer/packagist.go (search + metadata), error checks on every fmt.Fprint/Fprintf stub writer in registry_test.go, packagist_test.go and npm_test.go. Note: the capped scan showed 12 findings; golangci-lint's default max-same-issues=3 hides identical ones, so all 25 were fixed (verified with --max-same-issues=0); `go test ./...` passes
- [x] 3.5 Verify the full gate is green: `./lint.sh` exits 0 on the clean tree

## 4. Build gate

- [x] 4.1 Modify `build.sh` to run `./lint.sh` before `go build`, exiting non-zero without producing a binary when the gate fails; verify a clean-tree `./build.sh` still produces ./alldeps and a deliberately introduced finding (e.g. an unused variable) makes it fail before compilation
- [x] 4.2 Run `go test ./...` after the flip; verify all packages pass

## 5. Release step and docs

- [x] 5.1 Add the mandatory vuln-scan step to `docs/release.md` before the snapshot dry-run (run `./vuln.sh`; a reachable finding blocks the release until fixed or explicitly accepted); verify the doc order places the scan before snapshot/tagging
- [x] 5.2 Update AGENTS.md Setup Commands with Lint (`./lint.sh`) and Vuln (`./vuln.sh`), noting tools resolve to @latest at run time (no pins; refresh interactive binaries with `go install ...@latest` after Go upgrades); verify the documented commands match the scripts exactly
