## Why

Today the user must pick a single Node prefix and only ever see that prefix's packages. On a machine with many nvm versions this forces constant context-switching and hides the obvious question "where do I actually have this package, and is it consistent across my runtimes?" This change inverts the model for global mode: one unified list of every globally installed package across all destinations of the active manager, with a per-destination drill-down, and plans that can apply to any subset of destinations in a single run. It also introduces selecting *which* package manager operates in global mode (one at a time), laying the groundwork for yarn/pnpm.

## What Changes

- **Unified global list.** The list shows one row per package *name*, aggregated across all destinations of the active manager. Each row's headline is the copy in the highest-ranked destination that has it (rank = Node/runtime version, not the package version), plus a counter of how many other destinations also carry it.
- **Per-destination detail view.** A package's details include a traversable table of its presence per destination (installed version or absent) with `+`/`-` to add or remove it from a specific destination. This is also the surface later reused for Node dedupe/align.
- **Manager selection (global).** The user selects which package manager operates in global mode; exactly one is active at a time. Switching manager preserves installed state (shared per destination) and all pending marks, and does not re-list. The active manager is always visible in the status line.
- **Cross-destination plans.** Pending marks persist across destination and manager switches within a session. A single apply run dispatches every marked (destination, manager) group through its own manager. Locks are acquired for all destinations the plan touches before execution begins.
- **Install-target selection.** Marking an available package for install when more than one destination is eligible opens a popup to choose which destination(s); with a single eligible destination no popup appears and that destination is used.
- **One-manager-per-destination guard.** If marks would operate the same destination through two different managers in one plan, that destination's group is flagged as invalid and its operations are not executed until the user resolves it (distinct destinations are always allowed, even across managers).

## Capabilities

### New Capabilities
- `manager-selection`: selecting which package manager operates in global mode — exactly one active at a time, driven by detected/available managers, with installed state and marks preserved across switches and the active manager shown in the status line.

### Modified Capabilities
- `global-package-list`: the listing changes from "packages of the selected prefix" to "one row per package name aggregated across all destinations of the active manager," with a headline (highest-ranked destination that has it) and a cross-destination count, plus a new per-destination detail view.
- `package-operations`: plan preview and apply now span multiple (destination, manager) groups in one run; an install-target selection popup is added for multi-destination installs; a destination marked under two managers blocks that group's execution.

## Impact

- `internal/app` — list projection becomes cross-destination; new per-destination detail screen; manager switcher; apply engine generalizes from single-active-prefix to grouped dispatch; new install-target popup screen.
- `internal/domain` — mark storage is already per (destination, manager) from the seam change; plan grouping and the one-manager-per-destination invariant are exercised here.
- Status line format gains the active manager.
- Depends on `establish-ecosystem-seam` being applied first (the port, identity model, and plan invariant it introduces).
