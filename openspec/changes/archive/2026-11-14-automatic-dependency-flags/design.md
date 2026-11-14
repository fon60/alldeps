## Context

See proposal.md for motivation. The relevant current state:

- `internal/npmcmd.ParseLS` iterates only the top-level `Dependencies` of `npm ls -g --all --json`; the nested tree is deserialized but used solely to compute each root's `Broken` flag.
- `ecosystem.Package` carries `{Name, Version, Unhealthy}`; `domain.PkgState` carries per-package state plus `Marks`. Neither has an automatic/direct distinction.
- Flags are computed in the domain: `PkgState.StateChar()` (b/i/p) and `actionChar(Mark)` (`*`/`+`/`-`/`u`/`h`), combined by `FlagFor`/`UnifiedRow.Flag`. The view renders a 2-char cell padded to width 4, appending `!` for conflicts.
- The composer adapter currently lists only direct requires; the gomod adapter lists only direct requirements (indirect excluded). Both are specified that way today.
- Sorting by state compares `StateChar()`; several tests assert exact 2-char flag strings.

## Goals / Non-Goals

**Goals:**
- Surface every installed package (direct + automatic) as a row, marked with an `A` auto slot.
- Introduce the 3-slot `<state><auto><action>` flag and replace the `*` no-action marker with a space, consistently across list, search, versions, and plan surfaces.
- Make the automatic signal manager-agnostic (port + domain) so all adapters populate it.
- Provide a default-on "show all" view with a manual-only toggle.

**Non-Goals:**
- Changing what an `A` package means for planning/execution (automatic packages are installable/removable like any other; no new op kinds).
- Collapsing or grouping the dependency tree visually (rows stay flat, as today).
- Altering registry search semantics beyond flag rendering.

## Decisions

**D1 — Automatic is a per-row boolean on the port and domain, not derived in the view.**
Add `Automatic bool` to `ecosystem.Package` and `domain.PkgState`, populated by each adapter's `ListInstalled`. Rationale: the direct-vs-transitive fact is known only to the adapter (it owns the tree/build-list); deriving it in the view would require re-parsing manager output. Alternative considered — compute it in the app from a "parent" field — rejected because the port has no parent/depth concept and that would leak tree structure across the boundary.

**D2 — npm: recursive walk, dedup by name, top-level wins.**
`ParseLS` walks the full `lsNode.Deps` tree and emits one row per unique package name. A name present at the top level is direct (`Automatic=false`) even if it also appears nested; a name only reachable as a nested dependency is automatic (`Automatic=true`). The existing `Broken` computation (subtree walk) is preserved. Rationale: matches "installed automatically as a dependency" and avoids double-listing a package that is both a root and a transitive dep.

**D3 — composer: relax direct-only to full installed set, keep platform excluded.**
The composer adapter reads `vendor/composer/installed.json` (already parsed) and now emits every installed package; a package whose name is a key of `composer.json` `require` is direct, the rest automatic. Platform/virtual requirements (`php`, `ext-*`) remain excluded (they are not installed packages). Rationale: satisfies "all adapters" while preserving the one genuinely-correct exclusion.

**D4 — gomod: list the full build list, main module excluded, indirect = automatic.**
The gomod adapter now emits every module in the build list; modules marked indirect in the module graph are `Automatic=true`, direct requirements are not, and the main module is never listed. Rationale: consistent with D2/D3 and with the existing "resolved version from the module graph" behavior.

**D5 — 3-slot flag layout and rendering width.**
Flag = `<state><auto><action>`, each slot one char; empty auto/action render as a space. State precedence unchanged (b > i > p). The rendered cell widens from 2 to 3 base chars; the column is sized so the conflict `!` still fits after the flag (keep total width sufficient for 3 + `!`). Sorting by state continues to use the state slot only, so ordering is unaffected by the new slots. Rationale: minimal change to sort semantics while adding the auto dimension.

**D6 — Manual-only view is a dedicated toggle, not a filter token.**
A boolean in the app model (default false = show all) toggled by a key and surfaced in the status/hint bar; when true it filters out `Automatic` rows before rendering. It composes with the existing `f` filter expression. Rationale: the user described it as an on/off toggle and it is a common operation; a dedicated control is more discoverable than requiring `!~A`. Alternative considered — a `~A`/`!~A` filter token — kept available conceptually but not required; the toggle is the specified control.

## Risks / Trade-offs

- [Large lists] Surfacing all transitive deps can grow a list from dozens to hundreds/thousands of rows → cursor-following viewport already handles length; default "show all" is user-chosen; the manual-only toggle is the escape hatch.
- [Breaking flag strings] Tests and any downstream parsing that expect 2-char flags break → update all assertions to the 3-slot form in the same change; no external consumers (TUI only).
- [composer/gomod scope change is breaking] Their specs currently mandate direct-only listing → flagged **BREAKING** in the proposal; behavior change is intentional and user-approved.
- [npm `--all` cost] Walking the full tree for many packages increases parse work → parsing was already deserializing the full tree (only consumption changed), so no new I/O; CPU cost is bounded by what npm already returns.

## Open Questions

None that block specs or tasks. (Exact toggle key binding and the precise flag column width are implementation details to settle in tasks, not spec-level.)
