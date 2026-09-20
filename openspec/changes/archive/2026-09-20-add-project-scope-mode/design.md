# Design: add-project-scope-mode

## Context

See proposal.md for motivation. This change builds on `establish-ecosystem-seam`, which provides the `Ecosystem` port (including `DetectProject(root)` and `Capabilities().ProjectScope`), the (Ecosystem, Environment/destination, Name) identity, and per-(destination, manager) mark storage. It is independent of `add-global-unified-view`. Today startup always enters global mode targeting the active npm prefix.

## Goals / Non-Goals

**Goals:**
- A directory-based entry that selects project mode and roots it at the given path.
- Marker-file detection of applicable managers per project, presented via an adapter switcher (one active at a time).
- Strict per-adapter isolation of lists and plans, falling out of the existing identity keying.
- Reuse of the existing screens/state machine so project mode is a scoping difference, not a parallel UI.

**Non-Goals:**
- No new manager adapters are implemented here; detection reports what applies, but only npm (and whatever adapters already exist) actually function. composer/gomod adapters are later changes.
- No cross-project or multi-root management; one project per launch.
- No change to global mode behavior.

## Decisions

### D1: Mode selected from the single optional path argument
`main.go` parses an optional first argument. If present and it names an existing directory, the app starts in project mode rooted there; otherwise (no arg, or a non-directory) it either starts global mode (no arg) or rejects with a notice (bad path). No config file is introduced — the argument is the whole mechanism, matching the "npmitude ." intent.
- *Alternative: a `--project` flag* — rejected; a bare path argument is simpler and matches how tools like this are invoked.

### D2: Detection delegates to each adapter's DetectProject
The startup routine asks every registered adapter with `ProjectScope` capability to call `DetectProject(root)`. Each returns whether it applies and its tool variant (for the Node family, the lockfile type disambiguates npm/yarn/pnpm; a bare `package.json` defaults to npm). The union of positive results is the applicable-manager list. Detection is a pure filesystem read — no manager binary is spawned at open time.
- *Alternative: heuristic in the app* — rejected; each adapter owns the knowledge of its own markers, keeping detection correct as adapters are added.

### D3: Per-adapter isolation is a consequence of identity, enforced by scoping
Because a row's identity already includes the ecosystem, two adapters' same-named packages are distinct rows. The app scopes project mode to one active adapter at a time (analogous to manager selection in global mode): only the active adapter's collection is listed, and marks/plans are read from and written to that adapter's (destination, manager) namespace. Each Node/composer/gomod adapter uses a distinct destination (`node_modules`, `vendor/`, module cache), so there is no shared destination to leak across adapters.
- *Alternative: a single merged project list* — rejected; the user explicitly requires per-adapter separation to avoid name leakage.

### D4: Project environment is adapter-defined
The managed environment in project mode is the project, and each adapter defines its own destination for it (e.g. npm → `<root>/node_modules`). The domain's `Environment` carries this; the app does not hardcode layouts. This reuses the seam's "destination defined by the adapter" decision.

### D5: One top-level mode flag, shared screens
The `Model` gains a mode (global/project) and, in project mode, an active-adapter id plus the detected applicable list. All existing screens (list, detail, plan, info, versions, readme) are reused unchanged; only the environment set and the switcher differ from global mode. This keeps the change additive and avoids a second rendering path.

## Risks / Trade-offs

- [Detection false positives/negatives] → Mitigated by requiring marker files (not mere directory guesses) and defaulting Node-family to npm only when no lockfile is present; a project with no recognized markers in project mode shows an empty applicable list with a notice rather than guessing.
- [Only npm functions today, so composer/gomod paths are unexercised end-to-end] → The detection + switcher + isolation logic is adapter-agnostic and unit-tested against stub adapters reporting applicability; real composer/gomod behavior lands with their adapters.
- [Two modes increase startup branching complexity] → Kept small: one argument parse + one detection pass; the mode only selects which environment set and switcher to populate.

## Migration Plan

1. Parse the optional path argument in `main.go`; select global vs project mode (D1).
2. Implement the detection pass over `ProjectScope` adapters and build the applicable-manager list (D2).
3. Add project-mode top-level state (mode, active adapter, applicable list) and wire the adapter switcher (D3, D5).
4. Scope lists/plans to the active adapter's namespace; verify isolation (D3, D4).
5. Add a fixture-based test: a temp dir with `package.json`+`pnpm-lock.yaml` detects pnpm; a dir with `composer.json` detects composer; same-named package across two stub adapters stays isolated.

Rollback: revert in reverse order; global mode is untouched throughout.

## Open Questions

None. The exact set of marker files and lockfile names used for Node-family disambiguation will be confirmed against the real npm/yarn/pnpm outputs during implementation without changing the spec.
