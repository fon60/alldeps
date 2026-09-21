## 1. Model refactor (tabs + overlays)

- [ ] 1.1 Introduce `TabKind`, `Tab` (per-kind state), `Model.tabs []Tab` + `Model.tabIdx`, and the overlay classification replacing non-tab screens; move all per-screen fields into `Tab`; verify `go build ./...` compiles with dispatch updated
- [ ] 1.2 Implement tab operations — open (append+activate, subject-dedupe focus), close (remove, activate left neighbor, root uncloseable), move (Ctrl+h/l, Ctrl+arrows; inert at edges, under overlays/prompts, during apply) and verify with new unit tests covering open/dedupe/close/move/root-quit
- [ ] 1.3 Rework `updateKey` dispatch to the quitConfirm → prompt → overlay → tab order, delete `helpFrom`/`resolverFrom` bookkeeping, and verify existing screen-behavior tests (updated to the tab model) pass

## 2. Search as a tab

- [ ] 2.1 Move search state (query/hits/fetched/total/loading/cursor/listTop) into the Search tab; `/` on the list opens or focuses a Search tab, `/` on an active Search tab re-queries in place, esc/q closes the tab; verify with updated search tests plus new in-place re-query and close-discards tests

## 3. View: tab strip + per-tab bodies

- [ ] 3.1 Render line 3 of the header as the tab strip (full-width band, active tab brighter shade + bold, label truncation with the active tab always visible) and verify with a render test at 80x24 including an overflow case
- [ ] 3.2 Switch every body to `height − 3` and dispatch on the active tab kind / overlay; verify all screens still fit 80x24 via the existing render helpers (plan, info with many destinations, resolver)

## 4. Data scoping + apply overlay

- [ ] 4.1 Scope in-flight message guards (info doc, versions, readme, search pages) to the owning tab by (kind, subject), applying to inactive tabs that still match; verify with a stale-message test where the user moves away before data arrives
- [ ] 4.2 Keep the apply run as an exclusive overlay: no strip entry, tab keys inert during and after the run until dismissed, enter returns to the opening tab, q quits; verify with updated apply tests

## 5. Integration verification

- [ ] 5.1 Run `go build ./... && go test ./...` green, rebuild with `./build.sh`, and run an 80x24 pty session: list → info → versions → readme → back to list via Ctrl+h, re-open info (dedupe), open plan, close tabs — strip content and active shade correct at every step
