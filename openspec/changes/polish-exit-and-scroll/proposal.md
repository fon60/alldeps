# Proposal: polish-exit-and-scroll

## Why

Two surface defects degrade the app's feel: on quit the entire rendered interface is left on screen with the shell prompt appearing beneath it (the user wants to land back exactly where they were before launch), and scrolling down in the package list leaves the cursor row off-screen by one line — a direct violation of the existing "cursor row MUST always be visible" requirement in `global-package-list`.

## What Changes

- The program runs in the terminal's alternate screen buffer; on any exit (plain quit, quit after pending-marks confirmation, quit after an apply run) the pre-launch terminal content is restored pixel-perfect. New `app-shell` requirement.
- Fix the list viewport math: the list box height includes the column-header row, so visible data rows are box-height minus one; the scroll sync must keep the cursor inside that window, so scrolling down lands the cursor on the last printed row. The existing `global-package-list` viewport-scrolling scenario is clarified to pin this off-by-one.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `app-shell`: new requirement — terminal restoration on exit via the alternate screen buffer.
- `global-package-list`: "Viewport scrolling" clarified — at the bottom edge the cursor row is the last visible data row; the column-header line does not count as a data row.

## Impact

- `main.go` — one program option (`tea.WithAltScreen`).
- `internal/app/update.go` — `syncListTop` viewport math (and its use from `clampCursor`).
- `internal/app/view_test.go` — the existing viewport test encodes the off-by-one; expectations rewritten and a regression assertion added that the cursor row is actually rendered.
- No new dependencies; no rendering, keybinding, or other-screen changes.
