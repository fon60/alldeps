## 1. Terminal restoration on exit

- [x] 1.1 Add `tea.WithAltScreen()` to the program options in `main.go` and verify with a pty session (AGENTS.md pattern) that content written before launch is still visible after quitting and no interface residue remains

## 2. List viewport fix

- [x] 2.1 Change `syncListTop` in `internal/app/update.go` to treat visible data rows as `listHeight() − 1` (both edges and the max-top clamp) and verify `go test ./internal/app/` passes
- [x] 2.2 Rewrite `TestListViewportFollowsCursor` in `internal/app/view_test.go` for the corrected math (at 80x24: after 20 downs, cursor=20, top=5; rendered output contains the cursor row and not the first row) and add a bottom-edge regression test asserting the cursor row is the last rendered data row; verify `go test ./internal/app/ -run Viewport` passes

## 3. Integration verification

- [x] 3.1 Run `go build ./... && go test ./...` green, rebuild with `./build.sh`, and run an 80x24 pty session scrolling the list to the bottom then quitting — cursor row visible throughout and a clean exit
