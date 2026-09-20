## 1. Entry mode

- [x] 1.1 Parse an optional path argument in `main.go` and select project mode (existing directory) vs global mode (no arg), verified by a unit test that `npmitude .` selects project mode and no argument selects global mode
- [x] 1.2 Reject a path argument that is not an existing directory with a clear notice and no project-mode entry, verified by a unit test asserting the rejection path

## 2. Project manager detection

- [x] 2.1 Implement a startup detection pass that calls `DetectProject(root)` on every `ProjectScope` adapter and collects the applicable managers, verified by fixture tests: `package.json`+`pnpm-lock.yaml` → pnpm, `composer.json` → composer, `go.mod` → gomod
- [x] 2.2 Default the Node family to npm when a `package.json` is present with no recognizable lockfile, verified by a fixture test asserting npm is reported in that case
- [x] 2.3 Show a notice (empty applicable list) for a project directory with no recognized markers instead of guessing, verified by a pty run on an empty temp dir

## 3. Adapter switcher and scoping

- [x] 3.1 Add project-mode top-level state (mode, active adapter, applicable list) and an adapter switcher with exactly one active adapter, verified by a unit test asserting only one adapter is active and switching changes it
- [x] 3.2 Scope the list and plan to the active adapter's (destination, manager) namespace, reusing existing screens, verified by a unit test that the visible rows come only from the active adapter
- [x] 3.3 Preserve each adapter's installed state and marks across adapter switches, verified by a unit test that marks made under one adapter survive switching away and back

## 4. Per-adapter isolation

- [x] 4.1 Verify same-named packages across two adapters are never conflated (marking in one does not affect the other), verified by a unit test using two stub adapters both exposing a package named `helper`
- [x] 4.2 Verify a plan built and applied under one adapter executes only that adapter's operations, verified by a unit test asserting the other adapter's collection is untouched

## 5. Integration verification

- [x] 5.1 Run `go test ./...`, `./build.sh`, and a pty run of `npmitude .` in a fixture npm project (detect → switch → mark → plan), verifying detection, isolation, and the global-mode path all still work
