# Proposal: add-npmitude-tui

## Why

Managing globally installed npm packages today means ad-hoc CLI invocations (`npm ls -g`, `npm i -g`, `npm rm -g`) with no persistent overview, no discovery of what could be installed, and no safe preview of what a set of operations will do. The problem is multiplied for developers using version managers (nvm/fnm/volta): every Node version carries its own global package set, and nothing shows or tidies them together. `npmitude` brings aptitude's proven TUI model — browse, mark, preview, apply — to global npm packages.

## What Changes

- New greenfield CLI application `npmitude`, written in Go with a Bubble Tea terminal UI, distributed as a static binary with no host runtime requirement.
- Package list view of globally installed packages for the selected prefix, using aptitude-style two-character state/action flags (e.g. `i*` installed-no-action, `p+` available-marked-for-install), package name, size, installed version, and candidate version columns.
- Outdated detection: installed packages are compared against their `latest` dist-tag in the background so the candidate column and an "upgradable" filter are meaningful.
- On-demand registry search integrated into the list: the default view is the installed set only (no local package universe is maintained); searching queries the npm registry search API and results join the list as not-installed rows that can be marked for install.
- Multi-prefix support: detects Node prefixes from nvm, fnm, volta, and the system/active `npm config get prefix`; a switcher selects which prefix all operations target. All state (list, marks, outdatedness) is scoped per prefix.
- Mark-and-apply workflow: the user marks installs/removes/upgrades on list rows; applying shows a plan summary (packages to add/remove/upgrade, approximate download and freed sizes) and then executes via spawned `npm` subprocesses whose raw output streams directly to the terminal in a full-screen run view.
- Package info screen: description, dependencies, bin entries, license, maintainers, size, and available versions for the selected package, sourced from local `package.json` plus registry metadata; the package's README is viewable in a full-screen text view.
- Keyboard-driven aptitude-like keymap; the exact v1 key set is fixed in design.md (full `defaults.cc` parity is out of scope).

## Capabilities

### New Capabilities

- `global-package-list`: listing globally installed packages of the selected prefix with state/action flags, versions, sizes, and outdated detection against the registry `latest` dist-tag.
- `registry-search`: on-demand package discovery via the registry search API, with results merged into the list as not-installed rows; respects the configured registry URL.
- `prefix-management`: detection of Node installation prefixes (nvm/fnm/volta/system), selection/switching of the active prefix in the TUI, and scoping of all operations to it.
- `package-operations`: marking packages for install/remove/upgrade, plan preview before applying, and execution through spawned `npm` subprocesses with streamed progress output.
- `package-info`: the package detail screen showing description, dependencies, bins, license, maintainers, disk size, and available versions from registry metadata.

### Modified Capabilities

(none — greenfield project, no existing specs)

## Impact

- **Code**: entirely new codebase; a Go module is introduced at the repository root (the existing `package.json` remains only for OpenSpec tooling).
- **Dependencies**: Go toolchain; TUI libraries `bubbletea`, `lipgloss`, `bubbles`; registry access via the standard library `net/http` (no extra HTTP client needed).
- **Runtime requirements**: each managed prefix must contain its own Node + npm installation (writes run through that prefix's npm); npmitude itself has no host runtime requirement and runs regardless of the host Node version. Outbound HTTPS to the configured npm registry (read paths).
- **External systems**: reads registry metadata (search endpoint, package documents, dist-tags); mutates only the selected prefix's global `node_modules` via `npm`. No other system is touched.
- **Permissions**: writes to a system-owned prefix may require elevated privileges; handling is an open question (see design.md).
