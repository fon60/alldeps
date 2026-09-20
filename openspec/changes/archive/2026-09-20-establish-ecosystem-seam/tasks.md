## 1. Domain core extraction

- [x] 1.1 Rename `internal/state` to `internal/domain` (update all imports) and verify `go build ./...` succeeds and the existing state/sort tests pass unchanged
- [x] 1.2 Strip npm-specific fields (`Broken`, `NodeVersion`, `RegistryURL`) out of the shared row/environment types into adapter-supplied metadata with typed accessors, and verify `go test ./internal/domain/` passes with a unit test covering the generic health flag and version compare
- [x] 1.3 Encode the one-manager-per-destination plan invariant in the domain core (plan groups by destination; a destination mapped to two managers is reported invalid), verified by a unit test asserting npm@v16 + yarn@v18 is valid while npm@v18 + yarn@v18 is rejected

## 2. Ecosystem port

- [x] 2.1 Create `internal/ecosystem` defining the `Ecosystem` interface (Discover, ListInstalled, Search, Resolve, Execute, Lock, DetectProject, Capabilities) plus Intent/Plan/Conflict/ResolutionOption/Environment/Hit/Caps types, and verify `go build ./...` compiles the new package
- [x] 2.2 Give `Resolve` a contract returning `(Plan, []Conflict, error)` with a trivial default path (no conflicts), verified by a unit test that a conflict-free intent yields an empty conflict set

## 3. npm adapter

- [x] 3.1 Create `internal/adapter/npm` implementing the port by delegating to existing `npmcmd`/`registry`/`prefix`/`sizes` logic, and verify a unit test (fixtures + httptest stubs) exercises Discover/ListInstalled/Search against recorded npm JSON
- [x] 3.2 Move op-verb (`i -g`/`rm -g`) and layout (`<prefix>/lib/node_modules`, `bin/node`) strings out of `internal/app/model.go` into the npm adapter's Execute/Resolve, verified by grep showing no `"i", "-g"` / `node_modules` literals remain in `internal/app`
- [x] 3.3 Implement native-preferred Lock (npm has no global lock → fall back to the existing session lock), verified by a unit test asserting the fallback path is taken for npm

## 4. App rewiring + boundary

- [x] 4.1 Rewire `internal/app` to depend only on `domain` + `ecosystem`, injecting the npm adapter at startup, and verify `go build ./...` succeeds and no `app` file imports `npmcmd`/`registry`/`prefix` directly
- [x] 4.2 Add an import-boundary test asserting `internal/domain` imports nothing under `internal/adapter/...` or any TUI package, and `internal/app` imports no `internal/adapter/...` package, verified by the test failing when a deliberate violation is introduced then passing once removed
- [x] 4.3 Run the full suite (`go test ./...`) and `./build.sh` + `test/e2e-session.sh`, verifying all tests pass and the pty session (search → mark → plan → apply) behaves identically to before the refactor
