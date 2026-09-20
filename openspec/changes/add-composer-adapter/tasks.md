## 1. Adapter core

- [x] 1.1 Create `internal/adapter/composer` implementing the port for project scope: `DetectProject` on `composer.json`, `Capabilities` (ProjectScope, HasSearch, HasConflictResolution; no global/dedupe/native-lock), `Discover` returning the single project environment (project root, kind `composer-project`), verified by fixture tests for detection and discovery
- [x] 1.2 Implement `ListInstalled` from `vendor/composer/installed.json` showing direct dependencies (keys of `composer.json` require) with exact installed versions, empty list when vendor is absent, verified by unit tests against recorded installed.json fixtures including the no-vendor case
- [x] 1.3 Implement a Packagist client for search and latest-stable version lookup (skipping dev/alpha releases), verified by unit tests against local httptest stubs with no real network
- [x] 1.4 Implement `Execute` mapping install/upgrade to `composer require <name>:<constraint> --no-interaction` and remove to `composer remove <name> --no-interaction`, batched per op kind in the project dir with lifecycle scripts enabled, verified by a unit test using a logging shim asserting exact argv (no `--no-scripts`) and working directory

## 2. Conflict probing

- [x] 2.1 Implement the dry-run probe: run each pending op kind through `composer <op> --dry-run --no-interaction`, returning a normal plan on success and solver-failure text on non-zero exit, verified by a unit test using a fake composer shim that succeeds and fails respectively
- [x] 2.2 Implement problem classification into Conflicts with solver-derived ResolutionOptions (platform mismatch → compatible release line + skip; root-dep clash → pin candidates / remove dependency / skip; not found → corrected name / skip; unrecognized → fallback conflict carrying raw solver text), verified by table-driven unit tests against recorded solver outputs for every class including the fallback
- [x] 2.3 Verify option selection flows through the existing resolver loop: choosing an option updates the pending marks per its ResolutionEffect and triggers a background re-probe, verified by an app-level unit test with a stub ecosystem that reports a conflict, applies the chosen effect, and observes the re-resolve

## 3. Wiring and integration

- [x] 3.1 Register the composer adapter in project-mode startup wiring (global mode stays npm-only), verified by `go build ./...` succeeding
- [x] 3.2 Run `go test ./...`, `./build.sh`, and a pty run of `npmitude <fixture-composer-project>` covering detection → list with exact versions → marking a conflicting install → conflict marker + plan gate [Yes] → resolver options → apply executing the non-conflicting subset, verifying all pass and global mode + `test/e2e-session.sh` still work
