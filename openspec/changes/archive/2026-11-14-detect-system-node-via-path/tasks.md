## 1. PATH-scan discovery

- [x] 1.1 Add a helper that, given `$PATH`, yields candidate Node prefixes (for each dir, if `<dir>/node` or `<dir>/nodejs` is a regular file or symlink, the candidate prefix is the parent of `<dir>`); verify with unit tests over a fake PATH containing bin dirs with stub `node` and `nodejs` binaries.
- [x] 1.2 Validate each candidate by executing the found binary (`<dir>/<name> -p process.versions.node`, reuse `nodeVersionOf`) and drop candidates that fail to run; verify a non-executable candidate is excluded in a test.

## 2. Integration into Detect

- [x] 2.1 In `Detect`, after the nvm/fnm/volta scans, add PATH-discovered prefixes as `Source:"system"` entries deduped by resolved real path (`filepath.EvalSymlinks`) against all existing entries; verify a PATH entry inside an nvm dir produces no duplicate and two PATH aliases to one real prefix produce a single entry.
- [x] 2.2 Keep `npm config get prefix` as the sole source of the `Active` flag and default selection (merge logic unchanged); verify existing active-prefix tests still pass and a new test asserts the active env is unchanged when a system node is also on PATH.

## 3. Tests

- [x] 3.1 Extend `internal/prefix/detect_test.go` with fixtures for: system node outside VMs appears as `system`; node inside an nvm dir not duplicated; symlink dedup; active selection unchanged; verify `go test ./internal/prefix/` passes.

## 4. End-to-end verification

- [x] 4.1 Rebuild (`./build.sh`) and, on a machine with NVM active plus a system node on PATH, confirm the picker (E) lists the system node as an additional `system` environment while the nvm env remains the default selection; verify via a pty run showing both rows.
