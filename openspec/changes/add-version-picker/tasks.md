## 1. Data plumbing

- [x] 1.1 Extend the registry Doc with per-version unpacked sizes parsed from the npm document (`dist.unpackedSize`) and verify with an httptest fixture containing two versions with distinct sizes
- [x] 1.2 Implement gomod version enumeration (`go list -m -versions`) feeding `Doc.Versions` and verify with a fake-`go` shim test returning a fixed tag list, plus a failure-path test

## 2. Version list UI

- [x] 2.1 Rework the Versions tab body to the Flag/Version/Size/Where columns (newest-first ordering, headline+presence where column, size precedence) and verify with render tests covering: installed in several environments (+N), not installed anywhere (-), measured-size precedence over unpacked size
- [x] 2.2 Add cursor-following viewport state to the Versions tab (j/k plus g/G) and verify with a test that a longer-than-screen list keeps the cursor row visible at both edges

## 3. Entry points + pinning

- [x] 3.1 Bind v on the List and Search tabs to open/focus the Versions tab for the highlighted package and verify with unit tests (dedupe: pressing v twice focuses, does not duplicate)
- [x] 3.2 Confirm pin semantics unchanged (enter marks install/upgrade at the exact version on the active environment; the plan shows the pinned version) via existing plus new pin tests

## 4. Integration verification

- [x] 4.1 Run `go build ./... && go test ./...` green, rebuild with `./build.sh`, and run an 80x24 pty session: highlight a package with a long history, v, scroll to the bottom (cursor visible), pin a version, check the plan shows the pinned version
