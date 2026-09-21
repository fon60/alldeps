# Design: add-gomod-adapter

## Context

The Ecosystem port and project-mode wiring are in place: `DetectApplicable` consults every registered ProjectScope adapter, project mode builds one manager per applicable id, and the screens are ecosystem-agnostic (`main.go` already carries a comment reserving the slot for non-npm adapters). The npm adapter is the reference implementation. Go specifics that shape this design: MVS never fails on version conflicts; `go install` leaves no manifest (no global scope); the toolchain's own commands provide listing, update info, and mutation (`go list -m`, `go list -m -u`, `go get`).

## Goals / Non-Goals

**Goals:**
- A `gomod` adapter that lists direct module requirements with resolved versions, flags outdated modules via the toolchain, and applies install/upgrade/remove through `go get`.
- The app degrades gracefully where Go has no equivalent (search).

**Non-Goals:**
- Global mode for Go (no enumerable global tool scope).
- `go.work` multi-module workspaces (one module per project root in v1).
- `.go-version` / `toolchain` directive pinning (follow-up, mirrors the `.nvmrc` change).
- Conflict resolution (see D5) and automatic `go mod tidy`.

## Decisions

**D1: The Go toolchain is the source of truth for listing, outdated, and mutation.**
ListInstalled runs `go list -m -f '{{json}}' all` in the project dir and keeps entries with `Indirect=false`, excluding the main module; versions are the resolved build-list selections. LatestVersions runs `go list -m -u -f '{{json}}' all` as a background pass (TTL-cached like registry outdated checks) and reads each entry's `Update.Version`. Execute maps OpInstall/OpUpgrade to `go get <mod>@<version>` (or `@latest`) and OpRemove to `go get <mod>@none`, batched per op kind with the project dir as cwd. Rationale: resolved truth beats declared constraints, `-u` stays within the current major line for free (v2+ are separate module paths), and JSON output keeps us on the "never parse prose" rule. Alternative considered — parsing `go.mod` directly: offline and binary-free, but shows declared constraints rather than selected versions and needs a separate proxy client for outdated; rejected in favor of built-in tooling.

**D2: `Caps.HasSearch` is added to the port; the app gates search on it.**
Go has no official package-search API (pkg.go.dev is unofficial), so the adapter advertises `HasSearch=false`. The npm adapter sets the flag true. In the app, the search entry point checks the active manager's caps: when false, invoking it sets a "search not available for this ecosystem" notice and no query runs. Alternative considered — leave search always offered and let it fail: rejected, a permanently failing affordance is worse than an honest notice.

**D3: Operations are surgical; no automatic `go mod tidy`.**
`go get <mod>@none` drops the requirement but leaves stale go.sum entries; running tidy would also drop unmarked unused requires, which is surprising for a mark-driven tool. One mark = one effect; users run tidy themselves when they want graph cleanup.

**D4: ProjectScope only.**
`Caps.GlobalScope=false`: `go install pkg@v` places a binary in GOBIN/GOPATH with no manifest of what is installed, so there is nothing to list or manage globally. The adapter's Discover returns the single project environment (the project root, kind `go-module`); Writable checks the project directory.

**D5: No conflict resolution for Go.**
MVS resolves version clashes by construction (max of minimums), so the resolver has no conflicts to offer. The narrow operational failures that do occur (major-version path change on upgrade, toolchain requirement bump, retracted versions) are single-operation failures with at most two fixes — they surface through the existing apply flow's per-group failure notes and stay-marked behavior rather than being forced into Conflict/ResolutionOption shapes. `HasConflictResolution=false`.

**D6: Toolchain resolution is PATH-based.**
`go` is resolved from PATH (no prefix world like Node). Absence yields a clear load-failure notice ("Go toolchain required, not found on PATH"), the same failure class as npm's missing active prefix.

## Risks / Trade-offs

- [Cold module cache: first `go list -m all` touches the network] → listing is not first paint; a slow/failed initial list degrades to the load-failure notice path, retryable.
- [`-u` queries the proxy per module (chatty)] → background pass with TTL cache, identical pattern to registry outdated checks; never on the render path.
- [Missing/broken `go` binary] → clear notice per D6; all Go-project operations are unavailable but the app remains usable (global mode unaffected).
- [`go list -m -f json` field drift across Go releases] → we read only long-stable documented fields (Path, Version, Main, Indirect, Update.Version); a shape change fails parsing into a load-failure notice rather than corrupting state.

## Migration Plan

No migration: additive adapter behind an existing capability flag; global mode and npm behavior untouched. Rollback = revert the change.
