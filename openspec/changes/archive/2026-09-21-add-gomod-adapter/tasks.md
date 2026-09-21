## 1. Port and app search gating

- [x] 1.1 Add `HasSearch` to `ecosystem.Caps` and set it true in the npm adapter, verified by `go build ./...` succeeding and existing npm/app tests passing unchanged
- [x] 1.2 Make the app's search entry point honor `HasSearch`: when the active manager lacks it, invoking search sets a "search not available" notice and no query runs, verified by a unit test driving the search key against a stub ecosystem with `HasSearch=false` asserting the notice and no search command

## 2. gomod adapter

- [x] 2.1 Create `internal/adapter/gomod` implementing the port for project scope: `DetectProject` on `go.mod`, `Capabilities` (ProjectScope only, no search/dedupe/lock/conflicts), `Discover` returning the single project environment (project root, kind `go-module`), verified by fixture tests for detection (go.mod present/absent) and discovery
- [x] 2.2 Implement `ListInstalled` via `go list -m -f '{{json}}' all` in the project dir, keeping direct modules only (indirect and main excluded) with resolved versions, verified by a unit test using a fake `go` shim on PATH emitting recorded JSON with mixed direct/indirect/main entries
- [x] 2.3 Implement `LatestVersions` via `go list -m -u -f '{{json}}' all` reading each module's update version as the candidate, verified by a unit test against a fake shim whose JSON carries `Update.Version` fields
- [x] 2.4 Implement `Execute` mapping install/upgrade to `go get <mod>@<version>` (or `@latest`) and remove to `go get <mod>@none`, batched per op kind with the project dir as cwd, verified by a unit test using a logging shim asserting exact argv and working directory for all three op kinds
- [x] 2.5 Handle a missing `go` toolchain with a clear load-failure error naming the missing binary, verified by a unit test running listing with an empty PATH

## 3. Wiring and integration

- [x] 3.1 Register the gomod adapter in project-mode startup wiring (global mode stays npm-only), verified by `go build ./...` succeeding
- [x] 3.2 Run `go test ./...`, `./build.sh`, and a pty run of `npmitude <fixture-go-project>` (detection → list with resolved versions → mark upgrade → plan preview) plus a pty run asserting the search notice, verifying all pass and global mode + `test/e2e-session.sh` still work
