## 1. Automatic signal across the boundary

- [x] 1.1 Add `Automatic bool` to `ecosystem.Package` and `domain.PkgState`; thread it through the app's `toPkgStates` and the post-apply retained-row copy in `applyReloadMsg`; verify the build compiles and a unit test sets/reads the field.

## 2. npm full-tree listing

- [x] 2.1 Extend `npmcmd.ParseLS` to walk the full `lsNode.Deps` tree and emit one row per unique package name (top-level names direct, nested-only names automatic), preserving the existing `Broken` subtree computation; verify a fixture with nested deps yields all packages with correct Automatic flags via `go test ./internal/npmcmd/`.
- [x] 2.2 Map `Automatic` in the npm adapter `ListInstalled`; verify an adapter test asserts a transitive dependency is flagged automatic.

## 3. composer full installed set

- [x] 3.1 Change the composer adapter to emit every package from `vendor/composer/installed.json` (direct = name in `composer.json` require, else automatic), excluding platform/virtual requirements (`php`, `ext-*`); verify a list test asserts transitive packages are present and marked while platform reqs are absent via `go test ./internal/adapter/composer/`.

## 4. gomod full build list

- [x] 4.1 Change the gomod adapter to emit every module in the build list (indirect modules automatic, main module excluded); verify a list test asserts indirect modules are present and marked while the main module is absent via `go test ./internal/adapter/gomod/`.

## 5. 3-slot flag and rendering

- [x] 5.1 Rework domain flag computation to `<state><auto><action>` (state b/i/p; auto A/space; action space/+/-/u/h) and replace the `*` no-action marker with a space in `actionChar`; verify domain flag tests updated to 3-slot strings pass via `go test ./internal/domain/`.
- [x] 5.2 Update `UnifiedRow` flag/summary and keep state-based sorting on the state slot only; verify sort tests are unaffected by the auto/action slots.
- [x] 5.3 Widen the flag column in the view so three chars plus the conflict `!` fit on all surfaces (list, search, versions, plan) at 80x24; verify a pty run shows no truncation or overlap.

## 6. Manual-only toggle

- [x] 6.1 Add a model boolean (default false = show all) toggled by a key and shown in the status/hint bar; when true, filter out `Automatic` rows before rendering and compose with the `f` filter; verify a unit test asserts automatic rows are hidden when on and visible when off.

## 7. Verification

- [x] 7.1 Run `go test ./...` and fix any assertions that expect 2-char flags; verify the full suite is green.
- [x] 7.2 Rebuild (`./build.sh`) and confirm on a real nvm prefix that transitive deps are listed and marked `A`, the manual-only toggle hides them, and no-action rows show a blank rather than `*`; verify via a pty run plus visual check.
