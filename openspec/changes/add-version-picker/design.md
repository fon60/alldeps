## Context

The Versions screen exists today as a sub-screen of info: it renders `infoDoc.Versions` from index 0 into a fixed-height box (long lists clipped), marks only the active environment's installed version with `*`, and has no viewport state. `pinVersion` already marks install/upgrade at the exact selected version on the active environment. The npm registry document carries per-version `dist.unpackedSize`; the Doc struct does not retain it. The gomod adapter never populates `Doc.Versions`. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**
- One canonical, scrollable, environment-aware version list reachable from the list and from info.
- Full version data for all three ecosystems; graceful degradation on failure.

**Non-Goals:**
- No multi-environment pinning (a pin marks the active environment only, as today).
- No pre-download size estimates beyond registry unpacked sizes; no pagination of any kind.

## Decisions

- **D1 — Row layout.** Columns: Flag(4) Version(14) Size(8) Where(rest). Flag = state char (`i` if that exact version is installed in at least one environment, else `p`) + action char: the pending mark's action appears on the row whose version equals the mark's effective target — `TargetVersion` when set, the latest dist-tag for unversioned install/upgrade marks, the active environment's installed version for a removal mark; `*` elsewhere. Where = the headline+presence pattern reused from the unified list: highest-ranked environment carrying that exact version (node version in global mode, manager id in project mode) + `+N`; `-` when none.
- **D2 — Size sources, in precedence order.** (1) measured disk size when that version is installed in any environment (consistent with the main list's semantics); (2) npm registry `dist.unpackedSize` from the already-fetched document; (3) `…`. The registry Doc gains a per-version size map parsed for npm only; Composer/gomod leave it empty.
- **D3 — Viewport.** Reuse the corrected package-list math (box height minus title lines = data rows; cursor-following top). Versions tab state: subject, doc, cursor, top. j/k move; g/G jump to first/last for parity with the readme view.
- **D4 — Ordering.** Newest first everywhere, via the domain's existing version comparator, so "first row" is always the latest release regardless of upstream key order (npm document order is not contractual).
- **D5 — Entry points.** `v` on List (installed or search-hit row) and on Search opens/focuses the Versions tab for that package (subject = package name, deduped like any tab); the existing info-screen `v` is unchanged. Pinning keeps its current semantics (active environment only).
- **D6 — gomod enumeration.** `go list -m -versions <module>` prints all tagged versions space-separated; parse into `Doc.Versions`, sorted per D4. Runs through the adapter's existing go-invocation path with the project context; failures surface as the standard unavailability notice (non-fatal).

## Risks / Trade-offs

- [Unpacked size is not installed disk size] → measured size takes precedence whenever known; the column shows whatever source applies, no false precision.
- [`go list -m -versions` hits the network/proxy per open] → same class of cost as any registry fetch; result cached in the tab's doc like other fetched data; failure non-fatal.
- [Flag action-char rules are fiddly] → unit-test each mark shape (install@v, upgrade@v, install-latest, remove) against a fixed fixture.

## Migration Plan

None — additive UI plus adapter capability. Rollback = revert.
