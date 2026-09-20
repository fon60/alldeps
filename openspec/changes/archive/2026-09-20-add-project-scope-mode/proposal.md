## Why

Today npmitude only manages global installs. But several target ecosystems are fundamentally project-scoped — a composer project's `vendor/`, a Go module's dependencies, an npm/yarn/pnpm project's `node_modules` — and the user wants to manage those from the same aptitude-style interface. This change adds a project mode entered by pointing npmitude at a directory, auto-detects which package managers apply to that project, and lets the user operate on one manager's collection at a time with strict per-manager isolation.

## What Changes

- **Two entry modes.** `npmitude` (no path) launches global mode exactly as today. `npmitude .`, `npmitude ./<dir>`, or `npmitude /abs/dir` launches project mode rooted at that directory. A missing or non-directory argument is rejected with a clear notice.
- **Project manager detection.** On opening a project, the system detects which package managers apply to it from marker files: `package.json` (with lockfile type distinguishing npm/yarn/pnpm) for the Node family, `composer.json` for Composer, and `go.mod` for Go. The applicable managers are presented to the user.
- **Adapter switcher (project).** The user selects which applicable manager's collection to operate on at a time; exactly one is active. Switching preserves each adapter's installed state and marks.
- **Per-adapter isolation.** Lists and plans are strictly separate per adapter. A package name in one adapter is never conflated with the same name in another adapter, and no mark or plan operation leaks across adapters.
- **Project environment.** The managed environment is the single project (its per-manager module location), not a global prefix; all operations target that project.

## Capabilities

### New Capabilities
- `project-scope`: project mode — directory-based entry, marker-file detection of applicable package managers, an adapter switcher operating on one manager's collection at a time, strict per-adapter isolation of lists and plans, and a single project as the managed environment.

### Modified Capabilities
None. Global-mode behavior and the app chrome are unchanged; project mode is additive. (The earlier plan noted a possible `app-shell` touch, but its header/help requirements remain valid, so no delta is needed.)

## Impact

- `main.go` / startup — parse the optional directory argument and select global vs project mode.
- `internal/app` — new project-mode top-level state: active adapter, per-adapter list/plan scoping; reuses the same screens as global mode.
- `internal/ecosystem` port — `DetectProject(root)` (introduced in the seam change) is now exercised to build the applicable-manager list.
- `internal/domain` — identity already includes the ecosystem, so per-adapter isolation falls out of the existing keying; no new shared types required beyond scoping the active project.
- Depends on `establish-ecosystem-seam` (the port and identity model) and is independent of `add-global-unified-view`.
