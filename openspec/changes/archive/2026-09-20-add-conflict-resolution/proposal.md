## Why

Different package managers fail differently when packages clash: Composer and pacman surface dependency conflicts that force a choice (downgrade, remove, or skip), while the Node family carries redundant duplicate copies that waste disk. Today npmitude has no way to see or act on any of this — a bad plan just errors at apply time. This change adds package-level conflict resolution for every adapter: a non-intrusive marker on the list, an on-demand per-package resolver with explicit options and consequences, and a plan gate that lets the user decide whether to resolve before applying.

## What Changes

- **Conflict detection and list marking.** Packages involved in an unresolved conflict are clearly marked on the package list with a non-intrusive indicator. Marking never blocks navigation; the user chooses when to open resolution.
- **Per-package resolution screen.** Opening a conflicted package shows its conflicts, each offering resolution options (keep/use a version, downgrade, remove, or skip/not-install) with stated consequences — including an approximate size delta where relevant. Selecting an option updates the marks/plan and re-resolves.
- **Plan gate.** When the user opens a plan that contains unresolved conflicts, a popup asks whether to resolve them: **[Yes]** goes to the resolution screen; **[No]** shows the plan with the conflicting rows marked exactly as on the main list.
- **Apply with unresolved conflicts.** Applying executes only the non-conflicting operations and leaves the conflicting operations marked (not executed), recording a note about what was skipped.
- **Node dedupe/align flavor.** For the Node family, conflict resolution also surfaces redundant/duplicate package copies across destinations and offers align/consolidate/remove options to reduce disk usage, reusing the per-destination detail view.

## Capabilities

### New Capabilities
- `conflict-resolution`: package-level conflict handling for any adapter — non-intrusive list marking, an on-demand per-package resolver with options and consequences, a plan-open gate (resolve or continue), apply that proceeds with the non-conflicting subset, and a Node dedupe/align flavor.

### Modified Capabilities
None. The existing plan/apply requirements remain valid; the conflict gate and apply-gating are additive behaviors owned by this capability. (The earlier plan noted a possible `package-operations` touch, but no existing requirement's stated behavior changes, so no delta is needed.)

## Impact

- `internal/ecosystem` port — `Resolve(intent) -> (Plan, []Conflict)` (introduced in the seam change) becomes the source of conflicts; adapters populate `Conflict`/`ResolutionOption` with real content.
- `internal/domain` — stores per-package/destination conflict state and applies chosen resolutions to marks/plan.
- `internal/app` — new per-package resolution screen, a plan-open Yes/No gate popup, list marking for conflicted rows, and apply-time skipping of conflicting operations.
- Depends on `establish-ecosystem-seam` (the Resolve contract and Conflict types) and benefits from `add-global-unified-view` (the per-destination table reused by the Node dedupe flavor). Real conflict content for pacman/composer lands with those adapters; this change delivers the surface, gate, and Node dedupe.
