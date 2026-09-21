# Proposal: add-gomod-adapter

## Why

The `project-scope` capability already promises that a project with `go.mod` reports Go as an applicable manager, but today only the npm adapter exists — a pure Go project launches into "no recognized package manager". Go projects (a first-class part of this machine's workflow) are unmanageable in npmitude.

## What Changes

- New `gomod` adapter implementing the Ecosystem port for Go module projects (project scope only): detection via `go.mod`, listing and outdated detection through the Go toolchain itself (`go list -m` / `go list -m -u`), operations via `go get`.
- The port gains a `HasSearch` capability flag: Go has no official package-search API, so the adapter advertises `HasSearch=false` and the app hides registry search for such ecosystems (a clear notice instead of a permanently failing search). The npm adapter sets the flag to true.
- The gomod adapter is registered in project-mode startup wiring; global mode stays npm-only (Go has no enumerable global tool scope — `go install` leaves no manifest).

## Capabilities

### New Capabilities

- `go-module-management`: listing direct module requirements with resolved versions, built-in outdated detection, go-get-based install/upgrade/remove operations, and search-unavailable behavior for Go module projects.

### Modified Capabilities

(none — the project-scope detection requirement this change fulfills is already written; no spec text changes)

## Impact

- `internal/ecosystem` — `Caps` gains `HasSearch`.
- `internal/adapter/npm` — sets `HasSearch=true` (one line).
- `internal/app` — search entry point honors `HasSearch` (notice instead of failing search).
- `internal/adapter/gomod` — new package.
- `main.go` — registers the gomod adapter in project mode.
- No new dependencies; global mode and all other capabilities untouched.
