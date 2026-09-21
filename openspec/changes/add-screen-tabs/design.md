## Context

Today `Model` carries a single `screen` enum (list, picker, manager, targets, plan, info, versions, readme, resolver, help) plus flat per-screen fields (`cursor`, `listTop`, `infoName`, `infoDoc`, `verCursor`, `readmeScroll`, `helpScroll`, `searchQuery`, …). Search is not a screen at all — it is a mode of the list (`searchActive`). View and update dispatch on `m.screen`. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**
- A visible, navigable tab strip: open/close/focus/move with subject dedupe.
- Per-tab state isolation so returning to a tab restores it exactly.
- Minimal churn: overlays keep their current behavior; the apply run stays exclusive.

**Non-Goals:**
- No middle-click/drag reordering of tabs, no tab persistence across runs, no per-tab filters (the filter stays list-scoped as today).
- No mouse support for the strip.

## Decisions

- **D1 — Tabs vs overlays.** Tabs = contexts the user may want to keep and return to (List, Search, Info, Versions, Readme, Resolver, Help, Plan). Overlays = transient choices that block the context underneath (env picker, manager picker, install targets, quit confirmation, apply run). Rationale: everything in the tab set has a subject or independent state worth preserving; every overlay is a short decision loop. The apply run is an overlay despite being long-lived because it mutates the world and must not be wandered off mid-run (user-confirmed).
- **D2 — Data model.** `type TabKind int` (List, Search, Info, Versions, Readme, Resolver, Help, Plan); `type Tab struct { Kind TabKind; Subject string; … per-kind state }`; `Model.tabs []Tab`, `Model.tabIdx int`. Invariant: `tabs[0].Kind == List` and it is never removed. Per-kind fields move out of `Model` into `Tab`: cursor/listTop (List & Search), query/hits/fetched/total/loading (Search), name/doc/err/destCursor (Info), name/doc/cursor/top (Versions), name/lines/scroll (Readme), dest/name/cursor (Resolver), scroll (Help), gate (Plan). Non-tab screens become `Model.overlay` (none|picker|manager|targets); `quitConfirm` stays a bool; apply state stays on Model but is treated as an overlay for dispatch.
- **D3 — Subject identity and dedupe.** Search → query string; Info/Versions/Readme/Resolver → package name; Help and Plan → singletons (no subject). Open = linear scan for a (kind, subject) match → focus it; else append at the end + activate. User-confirmed: dedupe anywhere in the strip, not just the right neighbor.
- **D4 — Key routing order.** quitConfirm → prompt (esc cancels) → overlay (its own keys; esc/q closes it) → tab level: q/esc on a non-root tab closes it; Ctrl+h / Ctrl+ArrowLeft moves left, Ctrl+l / Ctrl+ArrowRight moves right (inert at strip edges, while an overlay/prompt is up, or during apply). Plain h/l are untouched — bare `l` remains "match installed" on the list. Rationale for Ctrl variants: bare l is taken, and Ctrl+H arriving as backspace only matters inside prompts, where tab keys are inert anyway.
- **D5 — Close semantics.** Closing removes the tab from the slice; `tabIdx` moves to the closed tab's left neighbor. Sub-screen pops disappear: closing Versions/Readme returns to whatever is left (normally Info). The `helpFrom` and `resolverFrom` bookkeeping is deleted — overlays and tabs return "to where I am" by construction.
- **D6 — In-flight data for inactive tabs.** Existing stale-message guards compare screen+name; they become (kind, subject)-scoped: an arriving doc/versions/readme/search page is applied to its owning tab as long as that tab still exists with the same subject — even if inactive — so returning finds it loaded.
- **D7 — Strip rendering.** Line 3 of the header, full-width primary-green band (color 28). Cells: `[label]` separated by single spaces; label = kind word + truncated subject (`List`, `Search "rea…"`, `Info react`, `Plan`). Active cell: background color 29 (slightly brighter green) with bold white text; inactive cells: dimmed text on the base band. If total width exceeds the terminal, labels truncate first and the window shifts so the active tab is always visible.
- **D8 — Layout budget.** Header 2 → 3 lines: every body computes `height − 3` instead of `height − 2`; at 80x24 the list data rows drop from 16 to 15 (after polish-exit-and-scroll's fix). All screens re-verified to fit at 80x24 (plan, info with many destinations, resolver tables).

## Risks / Trade-offs

- [Large refactor of a ~2000-line model with ~15 test files driving `m.screen` directly] → do it as one change with the existing suite as the safety net; update tests to the tab model (open-tab helpers) before deleting the `screen` field.
- [One line less of body height may break tight layouts at 80x24] → render-test every screen body at 80x24 in the suite (existing `render80x24` helper).
- [Ctrl+H/Backspace ambiguity in some terminals] → tab keys are only honored outside prompts; inside a prompt backspace behaves as before.
- [Plan tab staleness if marks change while it is inactive] → the plan renders live from marks on every View (derived state, no cache) — same as today's screen.

## Migration Plan

Single binary, no persisted state. Implement against the existing test suite; pty visual check of the strip at 80x24 before commit. Rollback = revert.
