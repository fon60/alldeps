# Design: add-global-unified-view

## Context

See proposal.md for motivation. This change builds on `establish-ecosystem-seam`, which provides: the `Ecosystem` port, the (Ecosystem, Environment/destination, Name) identity, installed state stored per destination with marks stored per (destination, manager), and the one-manager-per-destination plan invariant in the domain core. Today the app still operates on a single active prefix (`ActivePrefixID`) and lists that prefix's packages only.

## Goals / Non-Goals

**Goals:**
- A cross-destination unified list with a headline (highest-ranked destination) and presence counter, plus a per-destination detail view for targeted `+`/`-`.
- Global-mode manager selection (one active at a time) that preserves installed state and marks.
- An apply engine that dispatches a whole plan across multiple (destination, manager) groups in one run, locking all touched destinations first.
- An install-target popup for ambiguous multi-destination installs, and user-visible enforcement of the one-manager-per-destination rule.

**Non-Goals:**
- No second manager adapter is implemented here; the switcher lists whatever managers exist (npm today). yarn/pnpm adapters are later changes.
- No conflict-resolution UI (that is a later change); the plan-gate and per-package resolver surface are out of scope.
- No project mode (a separate change).

## Decisions

### D1: Cross-destination projection is a pure, tested function
The unified list is a projection over domain state: group installed packages by name across all destinations of the active manager; for each name pick the headline destination (max rank that has it) and count the rest. This is implemented as a pure function on the domain model (no TUI, no I/O) so it is unit-testable in isolation. The app calls it to build rows; rendering stays a pure function of the resulting rows.
- *Alternative: compute in the View* — rejected; the aggregation has real logic (headline selection, counting) that belongs with the data, not the pixels.

### D2: Headline rank reuses version comparison
" Highest-ranked destination" = max by `Environment.rank` (Node/runtime version), using the existing `CompareVersions`. A package present on v20/v18/v16 headlines at v20's copy. Ties or missing ranks fall back to a stable destination-ID order. This matches the confirmed interpretation that "newest" means the runtime version, not the package version.

### D3: Manager switcher is Model state; switching does not reload
`Model` gains `activeManagerID`. Switching manager changes which mark-set (per destination, per manager) is editable and which adapter executes operations; it does NOT clear or re-fetch installed state (shared per destination). Marks are keyed per (destination, manager), so they survive the switch for free. The status line renders the active manager alongside the active destination.

### D4: Apply engine generalizes to grouped dispatch
`buildApplyBatches` / `startApply` / `nextBatchCmd` are refactored from "single active prefix" to "iterate plan groups." A plan is the ordered set of (destination, manager) groups derived from all pending marks. Before running, the app acquires locks for every destination in the plan (native-preferred via the port), then executes each group through that destination's manager `Execute`, streaming output per group. Post-run reconciliation re-reads each touched destination's installed state. Invalid groups (one-manager-per-destination violations) are skipped and reported, not executed.
- *Alternative: one apply per destination* — rejected; the user explicitly wants a single plan to execute across destinations/managers in one run.

### D5: Install-target popup is a small modal screen
Marking an available package for install resolves eligible destinations (those of the active manager where the package is not already installed, subject to writability). If exactly one, record the mark immediately. If more than one, open a modal listing them with multi-select; on confirm, record one install mark per chosen destination. This reuses the existing prompt/modal machinery rather than adding a new screen class.

### D6: One-manager-per-destination surfaced, not silently dropped
The domain already reports an invalid (destination, manager-set) group. The plan preview renders such a group flagged as invalid with a short reason; apply skips it and notes it in the completion log. The user resolves by clearing one manager's marks on that destination (e.g. via the per-destination detail view or clear-all).

## Risks / Trade-offs

- [Cross-destination projection changes the list shape users know] → Mitigated by keeping the same flag columns and status line; only the row identity and two new columns (headline source, presence counter) change. Verified by pty run against a multi-version nvm layout.
- [Grouped apply is a non-trivial rewrite of the single-prefix path] → The seam change already isolated execution behind the port; this change swaps the loop driver. Covered by unit tests for group derivation + locking, and the e2e session test extended to a two-destination plan.
- [Locking many destinations up-front can hold locks longer] → Accepted; plans are short and single-flight, and holding all touched locks avoids mid-run contention. Stale recovery (from the seam) still bounds crash risk.
- [Manager switcher with one manager looks like a no-op] → Intentional; it is correct today and becomes meaningful when yarn/pnpm adapters land without further UI work.

## Migration Plan

1. Add the cross-destination projection + per-destination detail rows to the domain/app (D1, D2); unit-test headline/counter.
2. Add `activeManagerID` + manager switcher + status-line manager display (D3).
3. Generalize the apply engine to grouped dispatch with up-front multi-destination locking and invalid-group skipping (D4, D6).
4. Add the install-target popup (D5).
5. Extend `test/e2e-session.sh` to a two-destination plan; run full suite + pty visual check.

Rollback: each step is independently runnable; revert in reverse order.

## Open Questions

None. The exact status-line layout for showing both manager and destination will be finalized during implementation to fit 80x24 without changing the spec.
