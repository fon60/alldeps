## Context

See proposal.md for motivation. Current state: the versions tab (`updateVersions`, internal/app/update.go) handles only `j/k/g/G` and `enter`/`space` (pin at cursor on the active environment via `pinVersion`). There is no `+`/`-`. The closest existing UI is the install-targets overlay (`OverlayTargets`, `markInstallTargets`), a scrollable multi-select of eligible environments toggled with space and confirmed with enter, opened only when `+` on the list finds more than one eligible destination. Marks are stored per (destination, package, manager) in `PkgState.Marks`; `SetMarkFor` gives toggle semantics and `SetMarkEntry` force-sets a mark with a `TargetVersion`.

## Goals / Non-Goals

**Goals:**
- Add `+` (install-this-version) and `-` (remove) to the versions screen as orthogonal toggles.
- Reuse/generalize the existing multi-environment popup so both actions can target a chosen set of environments with `+`/`-`.
- Retire the enter/space pin so that only `+`/`-` mark on the versions screen.

**Non-Goals:**
- Changing mark storage or the plan/execution pipeline (marks flow into the existing plan as today).
- Adding bulk/version-range operations on the versions screen.

## Decisions

**D1 — `+` and `-` act on the version under the cursor with a pinned target.**
`+` sets an install (or upgrade-to-that-version) mark with `TargetVersion` = the cursor version; `-` sets a removal mark. Both are toggles: repeating the same key clears that mark. Rationale: matches the list/search mental model and the existing `MarkEntry.TargetVersion` mechanism used by pinning.

**D2 — Orthogonality is enforced in the handlers, not the data model.**
The `+` handler only ever creates/clears an install or upgrade mark and never issues a removal; the `-` handler only ever creates/clears a removal mark and never issues an install. `-` is a no-op (with a notice) when the package is not installed in any eligible environment. Rationale: directly implements the "plus never removes, minus never installs" contract at the single place where the keys are routed.

**D3 — Eligible-environment selection reuses existing logic.**
For `+`: environments where the package is not already present at that version and the environment is writable (same eligibility as `markInstallTargets`). For `-`: environments where the package is installed. Exactly one eligible environment → apply directly with no popup; more than one → open the popup. Rationale: consistent with the list's existing single-vs-multi behavior.

**D4 — Generalize `OverlayTargets` to a mode-aware, `+`/`-`-driven popup.**
Extend the targets overlay to carry a mode (install or remove) and treat `+` as include/select and `-` as exclude/cancel for the row under the cursor, keeping enter/space to confirm and esc/q to cancel. The existing list-install path keeps working (its space/enter bindings remain valid). Rationale: one popup serves both actions and both entry points; overlay keys are already intercepted before tab dispatch, so `+`/`-` inside the popup cannot leak to the versions screen.

**D5 — The enter/space pin is removed (Variant C).**
enter/space no longer perform a marking action on the versions screen; only `+`/`-` mark. Rationale: `+` already covers pin's single-active-environment install/upgrade case (and adds multi-env + toggle), and there is no reason to auto-return to the Info tab after one action — the user leaves with esc/q. This keeps one mental model across list/search/versions ("`+`/`-` mark, nothing else marks").

## Risks / Trade-offs

- [`-` on a not-installed row] Could confuse the user → D2 makes it a no-op with a notice rather than silently installing.
- [Popup key overlap] `+`/`-` now have meaning inside the popup → overlay handling runs before tab-level dispatch, so there is no double-handling; verified by the existing update pipeline ordering.
- [Toggle + target version interplay] Clearing an install mark must clear the whole mark (revert), not just strip the version → reuse `RevertFor`/`SetMarkFor` toggle rather than partial mutation.

## Open Questions

None that block specs or tasks.
