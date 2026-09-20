## 1. Pin matching

- [x] 1.1 Add a pure `MatchPin(pin string, versions []string) string` in `internal/prefix` (trim + optional leading `v`; 1–3 numeric dot components; full pin → exact match, partial pin → highest installed version with that dot-prefix; `""` when unparseable or unmatched), verified by unit tests covering exact, partial-to-highest, no-match, and unparseable (`lts/*`, empty) cases

## 2. Adapter binding

- [x] 2.1 Change `NewProject(root)` to `NewProject(ctx, root)`: read `<root>/.nvmrc` first line, run `prefix.Detect` once, bind the matched prefix (or the active prefix when no pin applies) as the project toolchain, and record a fallback notice when a parseable pin matches nothing, verified by unit tests with fake nvm layouts asserting the bound prefix for full/partial pins, the active-prefix fallback with a non-empty notice for an uninstalled pin, and no notice for absent/unparseable pins
- [x] 2.2 Make the project paths (ListInstalled, registry resolution, Execute) use the bound prefix instead of calling `prefix.Active` per operation, verified by a unit test asserting a project `ls` runs with the pinned prefix's node (fake layout logging its own path) rather than the active one

## 3. App wiring

- [x] 3.1 Surface the adapter's pin fallback notice as a non-blocking startup notice in project mode, verified by a pty run on a fixture project whose `.nvmrc` pins an uninstalled version showing the notice while the list loads normally

## 4. Integration verification

- [x] 4.1 Run `go test ./...`, `./build.sh`, and a pty run of `npmitude <fixture>` where `.nvmrc` pins a version that is installed (e.g. `22` against the real nvm layout), verifying project operations use the pinned toolchain, global mode still works, and all tests pass
