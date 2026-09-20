# Design: add-conflict-resolution

## Context

See proposal.md for motivation. This change builds on `establish-ecosystem-seam`, which defines `Resolve(intent) -> (Plan, []Conflict)` and the `Conflict`/`ResolutionOption` types in the domain core, and benefits from `add-global-unified-view`, whose per-destination detail table is reused by the Node dedupe flavor. Conflicts are produced by each adapter's resolver; npm's resolver reports none today (so the feature is dormant for npm except dedupe), while pacman/composer will populate real conflicts in their later adapters.

## Goals / Non-Goals

**Goals:**
- A manager-agnostic conflict surface: list marking, a per-package resolver with options + consequences (+ size delta), and a plan-open gate.
- Apply that proceeds with the non-conflicting subset and leaves conflicting operations marked (per the confirmed OP1 decision).
- A Node dedupe/align flavor reusing the per-destination table to cut redundant copies.

**Non-Goals:**
- No real conflict *content* for pacman/composer here — their adapters (later changes) supply the actual options/consequences. This change delivers the contract, surface, gate, and Node dedupe.
- No automatic resolution; every choice is user-driven.
- No modeling of conflicts that span the whole dependency graph beyond what a per-package option's consequence text can express.

## Decisions

### D1: Conflicts are adapter-produced; the surface is generic
The domain stores conflict state keyed by (destination, package), populated from the adapter's `Resolve`. The app renders one generic resolver screen for every manager; adapters decide which options exist and what their consequences/size deltas are. This keeps "available at package level for any adapter" true without a bespoke screen per manager.
- *Alternative: per-manager screens* — rejected; it would multiply UI and diverge over time. One surface, adapter-fed content.

### D2: List marking is a lightweight indicator, not a modal
A conflicted row gets a distinct marker in the existing flag column (or an adjacent glyph) plus inclusion in a conflict filter. It never interrupts navigation. This matches the "non-intrusive warning" requirement and keeps the list scannable.

### D3: Resolver screen is per-package and single-select per conflict
The screen lists each conflict for the package; each conflict offers its options (label, consequence, size delta). The user picks one option per conflict; the choice updates marks/plan and triggers a re-resolve so dependent conflicts refresh. This is the same screen reached from both the list marker and the plan gate.

### D4: Plan gate is a two-choice popup at plan-open
Opening a plan with unresolved conflicts shows exactly the confirmed popup: *"There are conflicts in the plan; would you like to resolve them?"* with **[Yes]** / **[No]**. Yes → resolver screen for the affected packages. No → the plan renders with conflicting rows carrying the same indicator as the list. The gate does not block viewing the plan; it only offers resolution up front.

### D5: Apply gating executes the non-conflicting subset
At apply time, operations involved in an unresolved conflict are excluded from execution and remain marked; all other groups/operations proceed normally (including the cross-destination dispatch from the unified-view change). A completion note lists what was skipped. This is the OP1 decision: partial progress, no silent drop, conflicting work stays visible for a retry.

### D6: Node dedupe reuses the per-destination table
For the Node family, a "conflict" can be redundancy (the same package at different versions across destinations, or duplicate copies in a tree). The resolver view renders these through the existing per-destination detail table (version + size per destination) and offers align/consolidate/remove options whose size deltas come from the already-measured sizes. No new data source is needed.

## Risks / Trade-offs

- [Conflict semantics differ sharply per manager] → Mitigated by keeping the surface generic and pushing all content to adapters; a manager that cannot express conflicts simply reports none (npm today).
- ["Perfect" resolution needs per-manager investigation] → Acknowledged: this change ships the contract + Node dedupe; pacman/composer option quality improves in their own adapter changes. The generic surface means those improvements need no UI rework.
- [Re-resolving after each choice can be costly for large plans] → Re-resolution is scoped to the affected package/destination and runs in the background with a notice, never blocking the UI (consistent with existing non-fatal enrichment behavior).
- [Applying a partial plan could leave a manager in an intermediate state] → Each destination's post-run truth re-read (from the seam) reconciles marks; skipped conflicting ops stay marked so the user can resolve and retry without losing intent.

## Migration Plan

1. Store conflict state per (destination, package) in the domain and surface it from `Resolve` (D1).
2. Add list marking + a conflict filter (D2).
3. Build the per-package resolver screen with options/consequences/size delta and re-resolve on choice (D3).
4. Add the plan-open Yes/No gate (D4).
5. Implement apply gating (non-conflicting subset, skip + note) (D5).
6. Add the Node dedupe/align flavor on the per-destination table (D6).
7. Unit-test with stub adapters that report conflicts; pty-run the gate and a Node dedupe scenario.

Rollback: revert in reverse order; without conflict content from an adapter the feature is inert, so it cannot regress existing apply behavior.

## Open Questions

None blocking. The exact marker glyph and the re-resolution concurrency limit will be finalized during implementation to fit 80x24 without changing the spec.
