# Design: establish-ecosystem-seam

## Context

See proposal.md for motivation. Current state that shapes this design:

- `internal/app/model.go` (~1100 LOC) mixes TUI rendering with plan-building (`buildApplyBatches` hardcodes `i -g`/`rm -g`), apply execution (`nextBatchCmd` builds `node npm-cli.js …`), and reconciliation. It imports `npmcmd`, `registry`, `prefix`, `sizes` directly.
- `internal/state` is the closest thing to a domain model but leaks npm concepts: `PkgState.Broken` (tree validity), `PrefixState.NodeVersion`, `PrefixState.RegistryURL`, and the identity `PrefixState.ID = absolute prefix path`.
- The original design (D3/D4) already *intended* an adapter seam and an "environment" keying concept; this change makes that intent real with an actual interface.

Constraints:
- Writes MUST go through the target environment's own manager so lifecycle/permission semantics stay exactly as the user's tool would do them.
- The app must stay a self-contained static binary; adapters may shell out to the ecosystem's own binaries but never become a host runtime dependency of npmitude.
- Behavior is frozen: this change is verified by existing tests passing unchanged plus a pty run that looks identical.

## Goals / Non-Goals

**Goals:**
- A dependency-free domain core that owns the workflow invariants and can be unit-tested with no TUI, no real package manager, and no network.
- One `Ecosystem` port that npm implements today and yarn/pnpm/composer/gomod/pacman implement later, without touching the core or the app.
- An identity model that supports both future modes (global cross-environment, project-scope) and the *one-manager-per-destination* plan rule.
- A verifiable import boundary so contributors cannot re-couple the app to a specific manager.

**Non-Goals:**
- No new user-facing feature. No unified global view, no project mode, no conflict UI — those are later changes built on this seam.
- No second adapter is implemented here; only npm. The port is shaped for N, but only one exists.
- No change to the lock's observable semantics (exclusive, refuse-don't-wait, stale recovery); only where the mechanism lives.

## Decisions

### D1: Package layout — `domain`, `ecosystem` (port), `adapter/<name>`

```
internal/
  domain/        dependency-free core (renamed from state/)
  ecosystem/     the Ecosystem port interface + intent/plan wire types
  adapter/
    npm/         implements Ecosystem; imports registry/, sizes/, node-detection
  app/           depends on domain + ecosystem ONLY
  registry/      node-registry HTTP client (npm-adapter implementation detail)
  sizes/         disk-size walk (shared infra, imported by adapters)
  lock/          session-scoped fallback lock (imported by adapters that lack a native one)
```

`state` is renamed to `domain` to signal the role change and to make the import-boundary test's target unambiguous. Alternatives considered:
- *Keep the name `state`* — less churn, but "state" reads as UI state; `domain` makes the zero-dependency contract legible to new contributors.
- *Put the port in `domain`* — rejected: the port is a boundary artifact that adapters implement and the app consumes; keeping it in its own package prevents `domain` from growing an interface that conceptually belongs to the edge.

### D2: The `Ecosystem` port

```go
type Ecosystem interface {
    ID() string                       // "npm" | "yarn" | ... stable, shown in UI
    Discover(ctx) ([]Environment, error)   // destinations this manager can operate on
    ListInstalled(ctx, env) ([]Package, error)  // what is actually present (per destination)
    Search(ctx, env, query string) ([]Hit, error)
    Resolve(intent Intent) (Plan, []Conflict, error) // intent -> concrete plan OR conflicts
    Execute(plan Plan) (stream of output, error)     // runs the real manager, raw passthrough
    Lock(env) (LockHandle, error)      // native-preferred; falls back to session lock
    DetectProject(root string) (Applicable bool, meta Meta)  // for project mode (later change)
    Capabilities() Caps                // e.g. HasConflictResolution, HasNativeLock, GlobalScope, ProjectScope
}
```

Rationale:
- `Resolve` returning `(Plan, []Conflict)` is the single most important contract: it is what lets conflict-aware managers (pacman/composer) produce a plan *or* surface conflicts, while npm's resolver is trivial (batch by op kind). Later changes build the UI on this without re-opening the seam.
- `Execute` streams raw output and does not parse it for correctness (preserves D3's raw-passthrough rule); truth comes from a post-run `ListInstalled`.
- `Capabilities()` lets the app adapt its UI (hide conflict controls for managers that lack them, hide project mode for global-only managers) without type-switching on the manager ID.

Alternatives considered:
- *Separate `Resolver` and `Executor` ports* — rejected for now; one port per manager keeps adapter modules self-contained and is easier for a solo maintainer to reason about. Can be split later if a manager needs a different resolve/execute lifecycle.

### D3: Identity — (Ecosystem, Environment, Name); destination vs manager

- `Environment` = a concrete **destination** (an opaque adapter-defined ID). For npm it is the Node prefix path; for project mode it is the per-manager module dir; for pacman it is the system DB. It carries a comparable rank (`NodeVersion` for Node) used later for ordering/headline logic, plus free-form `Meta`.
- A **destination can be shared across managers**: npm and yarn both target the same Node prefix. So installed state is stored **per destination** (one source of truth for what is physically present), while pending marks are stored **per (destination, manager)** because a mark means "do this via <manager>".

```
Environment { ID(destination), kind, rank, meta }
  Installed map[name]PackageState          // per destination, manager-agnostic
  Marks     map[ecosystemID]map[name]Mark  // pending ops per manager on this destination
```

This is what makes the *one-manager-per-destination* rule (see D4) expressible and what lets global mode switch managers without re-listing (installed state is shared; only the edited mark-set changes).

### D4: One-manager-per-destination plan invariant (domain rule)

A plan groups operations by destination. **A single destination MAY be operated on by at most one manager within one plan.** Violations (e.g. npm and yarn both marked against Node v18) make that destination-group invalid; the domain reports it so a later change can surface it. Distinct destinations are always compatible, even across managers (npm@v16 + yarn@v18 is fine; npm+composer+gomod is fine because their destinations never overlap).

This rule lives in the domain core now (it is an invariant of plan validity) even though the *user-facing* enforcement arrives with the cross-environment plan change. Keeping it in the core prevents a later change from having to retrofit it into shared code.

### D5: npm-specific fields become adapter metadata

`Broken`, `NodeVersion`, and `RegistryURL` leave the shared row type. The domain's `PackageState` keeps only manager-agnostic facts (installed version, latest-known version, size, mark, target version, a generic health flag). The npm adapter populates `Environment.Meta` / package-level metadata with Node-specific values; the app reads them through typed accessors guarded by `Capabilities()`, so no other manager's absence of these fields leaks into shared code.

### D6: Import boundary enforced by test

A Go test (e.g. in a `boundary_test.go` at module root or in `domain`) asserts, via `go/parser`/module inspection or a maintained allow-list, that:
- `internal/domain` imports no package under `internal/adapter/...`, and no TUI package.
- `internal/app` imports no `internal/adapter/...` package; it may import `internal/ecosystem` (the port) only.

This is the ArchUnit stand-in that protects the seam as the project grows contributors.

## Risks / Trade-offs

- [Rename `state`→`domain` touches many imports/tests] → Mechanical rename; verified by the full existing suite passing unchanged and `test/e2e-session.sh`.
- [Port is shaped for N managers but only npm exists, so parts are untested-by-use] → Mitigated by unit-testing the port contract against the npm adapter and by keeping the interface minimal (YAGNI on methods no current or near-term manager needs).
- [Storing marks per (destination, manager) complicates the current single-active-prefix code paths] → The app keeps its single-active-destination UX in this change; the richer mark storage is additive and exercised only trivially until later changes use it.
- [`Resolve` returning conflicts now, with no UI to show them] → npm's resolver returns an empty conflict set today; the return value is simply ignored by the current apply path until the conflict change lands. No dead behavior.

## Migration Plan

1. Rename `internal/state` → `internal/domain`; strip npm-specific fields into metadata (D5). Existing tests pass.
2. Add `internal/ecosystem` with the `Ecosystem` interface + intent/plan/conflict wire types (D2, D4).
3. Create `internal/adapter/npm` implementing the port by delegating to existing `npmcmd`/`registry`/`prefix`/`sizes`/`lock` logic; move op-verb and layout strings out of `app/model.go` into it.
4. Rewire `internal/app` to depend on `domain` + `ecosystem` only; inject the npm adapter at startup.
5. Add the import-boundary test (D6). Run full suite + `test/e2e-session.sh`; confirm a pty run is visually identical to before.

Rollback: each step is independently runnable and behavior-preserving; revert any step without affecting earlier ones.

## Open Questions

None. The package names in D1 are proposals that can be adjusted during implementation without changing the approach or the (later) specs.
