# Proposal: add-screen-tabs

## Why

The UI is a single screen stack with no visible trace of where the user has been: opening info, versions, readme, search, or plan replaces the view entirely, so depth is invisible and returning to an open context requires closing everything above it. A tab strip makes traversal visible and lets the user jump between open contexts without losing them.

## What Changes

- New tab model: persistent context screens become tabs — **List** (root, uncloseable), **Search "query"**, **Info \<pkg\>**, **Versions \<pkg\>**, **Readme \<pkg\>**, **Resolver \<pkg\>**, **Help**, **Plan**. Transient popups stay as overlays on the active tab and never appear in the strip: environment picker, manager picker, install-targets popup, quit confirmation, and the apply-run view (input stays exclusive for its whole duration).
- The header band grows from two lines to three; line 3 is the tab strip, full-width on the primary theme color. The active tab renders in a slightly brighter shade of green with a bold label.
- Opening a context appends a tab and activates it. If a tab with the same kind and subject already exists anywhere in the strip, the system focuses that tab instead of creating a duplicate (e.g. Info for the same package).
- `q`/`esc` on a non-root tab closes it (removed from the strip) and activates its left neighbor; `q` on the root List tab quits the program with the existing pending-marks confirmation. Ctrl+ArrowLeft/ArrowRight and Ctrl+h/l move between open tabs without closing any.
- Each tab owns its state (cursor, scroll offset, loaded data), so returning to a tab restores it exactly; the Plan tab re-renders live from current marks.
- Search becomes a first-class tab instead of a mode of the list: `/` on the list prompts and opens (or focuses) a Search tab; `/` on an active Search tab clears its results and takes a new query in place; `esc`/`q` on a Search tab closes it, leaving the installed list underneath untouched.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `app-shell`: header becomes three lines (title, screen actions, tab strip); new "Tab navigation" requirement covering open/close/focus/dedupe/move and root-tab quit.
- `registry-search`: results are presented in their own Search tab rather than replacing the list; `/` on a Search tab re-queries in place; closing the tab discards unmarked results.
- `package-info`: info, versions, and readme each open as their own tab with close/focus semantics.
- `conflict-resolution`: the resolver opens as its own tab; closing it returns to the previous tab (plan gate re-arms if conflicts remain).
- `package-operations`: the apply-run view is explicitly an exclusive overlay — it does not appear in the strip and blocks tab navigation for its duration.

## Impact

- `internal/app` — model.go/update.go/view.go refactor: a `tabs []Tab` slice with per-tab state replaces the flat `screen` enum plus per-screen fields; non-tab screens become an overlay classification. All app tests that drive screens directly are updated to the tab model.
- Every screen's body height shrinks by one line (header 2 → 3); layouts re-verified at 80x24.
- No new dependencies.
