# Proposal: add-version-picker

## Why

Manual version selection is today only reachable through the info screen, its list has no viewport (long histories are silently clipped and the cursor can move off-screen), it says nothing about which environments carry which version, and Go-module projects have no version history at all. The user wants to pick an exact version from the main list with a full, scrollable, environment-aware view.

## What Changes

- `v` on the main list opens the Versions tab for the highlighted package (in addition to the existing info-screen entry point); it also works on search-result rows.
- The version list is redesigned to resemble the main list, with columns: **Flag** (state `i`/`p` per exact version, plus the pending mark's action character on the row whose version the mark targets), **Version**, **Size** (measured disk size when that version is installed somewhere, otherwise the registry-reported unpacked size, otherwise an unknown indicator), and **Where** (the highest-ranked environment carrying that exact version plus a `+N` presence counter for further environments holding it; an absent indicator when installed nowhere).
- The list gets a cursor-following viewport (same math as the fixed package list) so arbitrarily long histories are navigable; enter pins the exact version under the cursor with the existing mark semantics.
- Versions are shown newest-first across all ecosystems.
- The gomod adapter gains published-version enumeration (`go list -m -versions`) so the Versions tab works for Go modules in project mode; the npm registry document additionally carries per-version unpacked sizes.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `package-info`: "Version history" is reworked — entry points (list + info), column layout with per-environment status, size sources, viewport scrolling, newest-first ordering; new requirement that every ecosystem provides the full published version list in one fetch, degrading gracefully on failure.

## Impact

- `internal/app` — Versions tab body/update (new columns, viewport state), `v` keybinding on List/Search tabs.
- `internal/registry` — Doc gains per-version unpacked sizes parsed from the npm document.
- `internal/adapter/gomod` — version enumeration feeding `Doc.Versions`.
- Tests: column rendering (env markers, +N, size fallbacks), viewport follow, pinning unchanged, gomod fake-`go` shim test, registry fixture with unpacked sizes.
- No new dependencies.
