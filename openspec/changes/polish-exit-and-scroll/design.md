## Context

`main.go` starts the Bubble Tea program with no options, so the UI draws in place on the main screen and everything it drew remains after exit. The list viewport math is split across two files: `view.go`'s `listRegion` renders a box of height `listHeight()` (= terminal height minus 7), one line of which is the column header; `update.go`'s `syncListTop` assumes all `listHeight()` lines are data rows. The existing unit test (`TestListViewportFollowsCursor`) was written against the implementation and encodes the off-by-one: it asserts `top=4` for cursor 20 at 80x24 and never checks that the cursor row is rendered, while the spec scenario demands it.

## Goals / Non-Goals

**Goals:**
- Quitting returns the user to exactly the pre-launch terminal state, on every exit path.
- The cursor row is always rendered; scrolling down lands it on the last visible row.

**Non-Goals:**
- No scroll-granularity changes (one row per keypress), no mouse or wheel support.
- No changes to the other scrollable views (readme, help) — their math is self-consistent within a single function and unaffected.

## Decisions

- **D1 — Alternate screen buffer (`tea.WithAltScreen`) over clear-on-exit.** Alternatives considered: emitting a clear sequence before quit (`tea.ClearScreenCmd` or raw escape codes) leaves a *blank* screen — the pre-launch content is gone from view, and every exit path would need its own handling. The alt buffer restores pixel-perfect with a single program option and covers all exit paths uniformly (bubbletea switches on start, restores on stop). Accepted trade-offs: terminal scrollback is not visible while running, and a hard-killed process can leave some terminals in the alt buffer (standard TUI risk, `reset` recovers) — identical to vim/less behavior.
- **D2 — Fix the scroll math, not the rendering.** Visible data rows = `listHeight() − 1`; `syncListTop` uses that for both edges and for the max-top clamp. Alternative considered: grow the rendered box by one line so all of it holds data rows — rejected because it would change what renders on every terminal size for a bug fix.

## Risks / Trade-offs

- [Alt buffer + SIGKILL strands some terminals in the alt screen] → standard TUI risk, recoverable with `reset`; accepted.
- [Existing test encodes the buggy expectation] → rewrite it (at 80x24: cursor 20 ⇒ top 5) and add an assertion that the rendered output contains the cursor row and not rows above the window, so the spec scenario is actually covered by the suite.

## Migration Plan

None — single binary, no persisted state. Rollback = revert the commit.
