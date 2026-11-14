# add-lint-gate — Design

## Context

Go 1.27.1 toolchain, single module (~18.5k LOC, 77 files), no CI. Project convention is root-level shell scripts (`build.sh`) plus AGENTS.md as the developer reference. Verified on this machine:

- `go vet ./...` — clean, ships with the toolchain (tracks the Go version by construction).
- Installed golangci-lint v1.60.3 (built go1.23) — refuses to run against Go 1.27.
- Installed staticcheck 2024.1.1 — cannot parse Go 1.27 export data ("unsupported version: 5").
- `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` (resolves v2.15.0, built with go1.27.4) — works; default linters report exactly 18 findings (errcheck 12, staticcheck 5, unused 1).
- `gofmt -l` — 16 files not clean (trivial comment alignment, mostly tests).
- `go run golang.org/x/vuln/cmd/govulncheck@latest` (resolves v1.9.0) — works; current tree has no known reachable vulnerabilities.

See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**

- One local lint command that fails the build on any finding.
- Tools that track the local Go toolchain automatically — no version strings to maintain anywhere in the repo.
- Mandatory reachability-based CVE scan as a pre-release step.

**Non-Goals:**

- SonarQube (any form) — deferred explicitly.
- CI integration — no CI exists for this repo.
- Enabling non-default linters ("all rules" mode) — deliberately deferred to a separate proposal; this change runs the default set + gofmt.
- Standalone staticcheck step in the gate — golangci-lint already executes it.
- Adding `go test ./...` to the build gate — deferred; lint only, per request.
- Any go.mod/go.sum dependency change — tools are dev tooling, not app dependencies.

## Decisions

### D1: Gate resolves tools at run time via `go run pkg@latest`

`lint.sh` runs `go vet ./...` then `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...`; `vuln.sh` runs `go run golang.org/x/vuln/cmd/govulncheck@latest .`. No version appears in any repository file. When the Go toolchain is upgraded, the next run automatically picks up the first compatible tool release — which is exactly the failure mode this change exists to eliminate (v1.60.3 / 2024.1.1 dead on Go 1.27).

Alternatives considered and rejected:

- **Pinned `go run pkg@vX`** — reproducible, but does not track the toolchain; every Go upgrade risks a broken gate until someone edits the script. Measured: even pinned `go run` performs a proxy metadata lookup (deprecation check) on Go 1.27, so it is not meaningfully more offline than `@latest`.
- **tools.go / go.mod dev tools** — pins move to go.mod and update only via manual `go get`, the opposite of the tracking goal. Measured cost of adding all three tools: go.mod 33→232 lines, go.sum 52→1005, and MVS force-bumps app-linked deps (x/sys v0.38→v0.49, x/text v0.3.8→v0.41) — the gate tooling would silently determine what the app links against. A bare `go get` without a tools.go file is pruned by `go mod tidy` (verified).
- **Installed binaries (`go install`) backing the gate** — fully offline once installed, but the version freezes at install time: this is precisely how the current broken state came about.

### D2: golangci-lint v2 as the single aggregator; no standalone staticcheck pass

The default linter set already runs staticcheck, govet, errcheck, and unused — verified by the fact that ST*/SA*/S*/QF* findings appear in its output. A separate `staticcheck ./...` step would re-run the same rules with no different result (modulo version skew) and double scan time.

- Alternative considered: run standalone staticcheck in the gate as defense-in-depth. Rejected as pure duplication; a refreshed standalone staticcheck remains available for interactive use (D6).

### D3: `go vet` stays an explicit first step in lint.sh

Cheap, toolchain-native, fails fast before the heavier aggregated run, and was explicitly requested. It overlaps with govet inside golangci-lint; the redundancy is deliberate for fail-fast ordering and explicitness.

### D4: Default linter set + gofmt, zero exclusions

`.golangci.yml` is minimal:

```yaml
version: "2"
formatters:
  enable:
    - gofmt
```

- The default set (errcheck, govet, ineffassign, staticcheck, unused) is relied on as-is rather than enumerated, so linters golangci-lint adds to its defaults later reach the gate automatically — consistent with the project policy of wanting upstream signals as early as possible.
- `gofmt` is enabled because it is not part of the default set (verified: the initial scan reported none of the 16 unformatted files). In v2 gofmt lives in the separate `formatters` section — putting it under `linters.enable` passes `config verify` but fails at run time with "gofmt is a formatter" (verified against v2.15.0); the YAML above is the corrected, working form.
- No errcheck exclusions. The 12 errcheck findings are real and get fixed in the one-time cleanup rather than silenced: lock.go's unchecked Close/Remove encode self-healing assumptions worth making explicit (e.g. lock.go:100 returns success even if Close fails after writing the lock file; lock.go:111 reports "contention" when the real failure is an unlink), registry.go's deferred body closes become explicit close + error merge (a Close error can indicate a truncated response), and the test stubs check their writes.
- Note: golangci-lint v2 uses a different config schema than v1 (top-level `version: "2"`); the exact YAML is validated with `golangci-lint config verify` at implementation time — do not copy v1 configs from online examples.

### D5: govulncheck in source mode, not binary mode

`vuln.sh` runs `go run golang.org/x/vuln/cmd/govulncheck@latest .` (source mode). Release binaries are built with `-ldflags="-s -w"`, which strips DWARF; binary-mode analysis degrades without it. Source mode builds its own unstripped copy and performs full call-graph reachability — it reports vulnerabilities actually reachable from alldeps code, not merely present in the dependency graph, and covers stdlib CVEs that lockfile scanners miss. `@latest` is doubly right here: the tool's value decays with both its own age and vuln-DB staleness.

### D6: Installed binaries refreshed once for interactive use only

`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` and `go install honnef.co/go/tools/cmd/staticcheck@latest` repair the broken PATH tools for interactive use. They back no gate step, so they carry no maintenance obligation; after a Go toolchain upgrade they are re-refreshed the same way if interactive use matters.

### D7: Vuln scan is a pre-release step, not part of the build gate

Different semantics from code quality: results depend on vulnerability-database freshness, so gating every build on it would couple builds to data-freshness and network availability. Instead `docs/release.md` makes the scan a mandatory step before the snapshot dry-run; a reachable finding blocks the release until fixed or explicitly accepted.

## Risks / Trade-offs

- [Upstream release changes gate behavior — new rule fires with zero local change] → desired per project policy: early signal beats a stable gate. Response when it happens: fix the finding or adjust config — never re-introduce version pinning. This policy is recorded in AGENTS.md.
- [Every run needs proxy metadata (version resolution / deprecation lookup)] → accepted: constant internet access is available, and the no-external-vendor rule targets paid SaaS infrastructure, not open-source module downloads. No account, key, or service endpoint involved.
- [Short window after a new Go release where @latest is still built with the older Go] → self-resolving once upstream ships a rebuild (typically days); until then `go vet` and the toolchain itself remain fully functional.
- [Reachability analysis can miss dynamically constructed call paths] → accepted; it is the strongest local option, and the release doc frames the scan as a check, not a guarantee.

## Migration Plan

Single change, ordered so the gate lands green:

1. Refresh installed golangci-lint and staticcheck for interactive use (environment fix, no behavior change).
2. Add `.golangci.yml`, `lint.sh`, `vuln.sh` (additive; nothing calls them yet).
3. One-time code cleanup: gofmt 16 files, fix all 18 findings (5 staticcheck, 12 errcheck incl. lock.go error handling and registry body closes, 1 dead function).
4. Flip `build.sh` to run the gate before compilation.
5. Add mandatory vuln-scan step to `docs/release.md`; update AGENTS.md Setup Commands.

Rollback: revert the `build.sh` change (gate off); scripts and config are additive and harmless if left in place.
