## Why

The package list shows only top-level (directly installed) packages; the dependencies pulled in automatically are invisible. Users want aptitude-style visibility into what was installed as a dependency, clearly marked, and a clearer flag grammar.

## What Changes

- Surface the full dependency tree, not just roots: every installed package becomes a row; non-top-level packages are marked automatic (`A`).
- Add an `Automatic` field to the manager-agnostic port (`ecosystem.Package`) and domain state (`domain.PkgState`); **all adapters** (npm, composer, gomod) populate it.
- Adopt a 3-slot flag `<state><auto><action>`: state ∈ {i,p,b}, auto ∈ {A, space}, action ∈ {space,+,-,u,h}. The `*` "no pending action" marker is replaced by a space. **BREAKING** to flag display, sorting, and tests that assert 2-char flags.
- Render the new flags on all package surfaces (list, search, versions, plan) via shared rendering; the versions-page spec delta is captured under `versions-page-marking`.
- Show all packages (manual + automatic) by default; add a toggle (default off) to show only manually installed packages.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `global-package-list`: rewrite "State and action flags" to the 3-slot form with the `A` marker and space-for-no-action; new requirement for automatic-dependency visibility (default all, manual-only toggle).
- `composer-package-management`: **BREAKING** — "Installed package listing" now includes transitive/automatic packages (marked `A`), not only direct requires.
- `go-module-management`: **BREAKING** — "Module listing from the build list" now includes indirect modules (marked `A`), not only direct requirements.

## Impact

- `internal/npmcmd` (recursive tree walk in `ParseLS`), `internal/ecosystem` (`Package.Automatic`), `internal/domain` (`PkgState.Automatic`, 3-slot flag computation, sorting), `internal/adapter/{npm,composer,gomod}` ListInstalled, `internal/app` (view rendering, sort, filter/toggle), and tests asserting the old 2-char flags.
