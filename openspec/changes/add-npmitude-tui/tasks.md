# Tasks: add-npmitude-tui

## 1. Scaffold & Tooling

- [x] 1.1 Initialize the Go module (`go mod init`, build script producing the `npmitude` binary) and verify `go build ./...` succeeds and `go run .` launches a minimal Bubble Tea app that renders placeholder text and exits cleanly on `q`
- [x] 1.2 Add TUI deps (`bubbletea`, `lipgloss`, `bubbles`) and verify `go mod tidy` resolves them and one trivial passing test runs via `go test ./...`
- [x] 1.3 Build the three-region layout shell (list region, description/info region, status line + prompt line) as Bubble Tea/lipgloss views with placeholder content and verify all four regions render correctly in an 80x24 terminal

## 2. State Model & Installed List (offline core)

- [x] 2.1 Implement the Bubble Tea state model (prefixes map keyed by prefix path, activePrefixId, filter, sort, applying) with pure update functions and verify unit tests cover mark set/toggle/clear and per-prefix isolation of marks
- [x] 2.2 Implement prefix tree loading by spawning `npm ls -g --json` for the selected prefix and parsing it into PkgState entries (name, version, broken flag from invalid/missing deps) and verify unit tests against fixture JSON outputs including a missing-dependency case, plus a manual check that launch on a real nvm prefix lists exactly its globals
- [x] 2.3 Render list rows with state/action flags, name, size (progressive "…" until measured), installed version, and candidate columns and verify manually that rows match `npm ls -g` output for the active prefix
- [x] 2.4 Implement background disk-size measurement by walking the prefix's global module directory and verify sizes appear after load and match `du -sh` per package within tolerance
- [x] 2.5 Implement the filter expression parser (`~i`, `~u`, `~b`, `~n <pattern>`, `!`, `&`, `|`) with recursive-descent evaluation and verify unit tests cover valid combinations, name patterns, and rejection of invalid expressions while retaining the previous filter
- [x] 2.6 Implement sorting by name (default), version, size, and state with a visible sort indicator and verify manually that each sort orders rows correctly
- [x] 2.7 Render the status line (prefix path, active filter, visible count, pending-mark count) and verify counts update live as marks are added or cleared
- [x] 2.8 Implement outdated detection via parallel `latest` dist-tag fetches with a concurrency limit and TTL cache that never blocks first paint and verify unit tests against a local `httptest` registry stub, a manual check that the candidate column fills in after network load, and an offline launch that renders the full list with a non-fatal notice
- [x] 2.9 Implement transient local name matching (live-narrows the visible list to installed packages while typing, zero registry access, cancel restores the full list) and verify manually offline that typing narrows rows with no network activity and cancelling restores them

## 3. Registry Search

- [x] 3.1 Implement search input (per design D8: aptitude-style prompt submitted on Enter) querying the search endpoint of `npm config get registry` with a page cap and verify unit tests for URL construction against a custom registry plus a manual check that a real query returns rows with state flag `p`, latest version, and short description
- [x] 3.2 Merge search results into the list by package name without duplicating already-installed rows and verify manually that searching for an installed package adds no duplicate row
- [x] 3.3 Handle search failures (network error, registry error, zero matches) with a message while leaving rows, filters, and marks untouched and verify manually with the registry made unreachable (e.g. bogus proxy env) that list state is unchanged

## 4. Prefix Detection & Switcher

- [x] 4.1 Detect prefixes from nvm/fnm/volta install directories plus the active npm prefix, each with Node version and global package count, and verify unit tests against fixture directory layouts plus a manual check on this machine (nine nvm versions) that all are listed with correct counts
- [x] 4.2 Default the selected prefix to the active npm prefix on launch and verify the status line shows its path immediately
- [x] 4.3 Implement the prefix switcher screen (list, select, switch) that reloads list and upgradability state for the new prefix and verify manually that switching between two nvm versions shows each one's globals
- [x] 4.4 Preserve pending marks per prefix across switches and verify manually the spec round-trip: mark two packages on v24, switch to v22, switch back, marks intact
- [x] 4.5 Handle vanished prefixes by dropping them on refresh and falling back to the active npm prefix with a notice when the selected one disappeared and verify manually by removing an nvm version outside npmitude, refreshing, and observing fallback plus notice

## 5. Mark / Plan / Apply

- [x] 5.1 Implement marking operations on the selected row (install, remove, upgrade-to-latest, hold, revert) updating the action flag immediately and verify manually each flag transition including toggle-off back to `*`
- [x] 5.2 Implement bulk operations (mark all upgradable respecting held packages; clear all marks) and verify manually the spec scenario: three of five outdated with one held yields exactly three upgrade marks
- [x] 5.3 Implement the plan preview screen listing installs (name, target version, approximate download size), removals (name, freed space), and upgrades (from→to) with explicit confirm/cancel and verify manually that cancelling leaves all marks pending and executes nothing
- [x] 5.4 Implement apply execution as batched invocations of the target prefix's own node + npm (grouped by operation kind) in a raw-passthrough run view (interface suspended, unfiltered output to terminal, completion prompt offering return or quit), with a single-flight guard, and verify end-to-end by installing a real package (e.g. `pnpm`) on a non-active nvm prefix, reading its raw output, returning via the prompt, and seeing the list refresh show it installed; verify a second apply attempt during the run is refused with a notice
- [x] 5.5 Re-read actual on-disk state after every apply run instead of assuming plan success and verify manually by forcing one operation to fail (e.g. uninstall while offline) and confirming the list shows exactly what is installed plus npm's diagnostic message
- [x] 5.6 Report permission failures on non-writable prefixes as a clear actionable message rather than raw npm output and verify manually against a root-owned prefix without sudo
- [x] 5.7 Implement the session-scoped exclusive environment lock (acquire on open, acquire-new-then-release-old on switch, release on exit; refuse when held with holder identification; stale recovery via holder liveness and start time; locks under `$XDG_STATE_HOME/npmitude/locks`) and verify unit tests for stale detection plus a manual check that a second npmitude instance is refused when opening an environment the first has open

## 6. Info Screen

- [x] 6.1 Implement the info screen showing name, version, description, homepage/repository, license, maintainers, bin entries (installed), disk usage (installed), and direct dependencies with version ranges, rendering absent fields as missing without breaking layout and verify manually on one package with full metadata and one without a description
- [x] 6.2 Distinguish dependency types in the listing (dependencies vs peerDependencies under separate labels) and verify manually on a package declaring both
- [x] 6.3 Implement the version history view from registry metadata with the installed/latest version identifiable, and selecting a specific version to pin the install mark to that exact version and verify manually that applying a pinned older version installs exactly that version
- [x] 6.4 Degrade the info screen to local data plus an unavailability notice when registry metadata cannot be fetched and verify manually with the network cut for a not-installed package
- [x] 6.5 Implement the full-screen README view (local README file for installed packages, registry readme field otherwise, plain markdown-to-text rendering, absent-README notice) and verify manually on an installed package with a local README, a not-installed package fetched from the registry, and one with no README

## 7. Keymap & Hardening

- [x] 7.1 Implement the v1 keymap per design D8 (navigation, `+ - = :`, `U x`, `/ S f`, `Enter d v C`, `u e E g Q`) with a hint bar in the status line and verify a manual walkthrough of every binding
- [x] 7.2 Harden layout for small terminals (<80 columns), long package names, and very long descriptions and verify manually by resizing the terminal through several sizes without crashes or broken rendering
- [x] 7.3 Write an end-to-end session script (launch → search → mark install → confirm plan → verify installed) runnable against a real nvm prefix and verify it passes from a clean checkout
- [x] 7.4 Implement the quit confirmation prompt (shown on any quit attempt, protecting unapplied marks) and verify manually that quitting requires confirmation and cancelling returns to the list

## 8. Manual Testing Feedback (Round 1)

- [x] 8.1 Fix npm ls parsing of string-valued `invalid` fields (platform-skipped optional dependencies, e.g. per-platform binary packages) so such prefixes load without error and the package is not flagged broken; verify unit test plus the real v16 prefix that previously failed
- [x] 8.2 Add the two-line header to every screen: line 1 app name + version @ host name, line 2 the actions available on the current screen (per-screen hint text)
- [x] 8.3 Implement the `?` help screen listing all key bindings grouped by context (scrollable), closing with esc/q/enter returns to the opening screen; verify from the list and from a sub-screen
- [x] 8.4 Highlight the whole cursor row across the full list width (all columns, contiguous) in the package list and environment picker; verify via unit test measuring the reverse-video region at 80 columns
- [x] 8.5 While a search query is active show only that search's results in the list (installed matches rendered as their authoritative installed rows), with esc clearing the search and restoring exactly the installed list; verify unit tests plus pty sessions for both not-installed and installed-match queries

## 9. Manual Testing Feedback (Round 2)

- [x] 9.1 Fix the list-screen frame being one line too tall (off-by-one in the height budget), which scrolled the terminal and hid the header title on the main list only; verify with a unit test asserting View() returns exactly `height` lines with the title first at several sizes
- [x] 9.2 Give the two-line header a primary-color (orange) background band with dark text on both lines; verify via unit test on the rendered SGR codes
- [x] 9.3 Implement search result pagination: page fetches via the registry `from` offset, infinite scroll when the cursor reaches the end of loaded results, loaded/total counts in the status line (registry `total` metadata), single-flight guard and recoverable failed page fetches; verify unit tests plus a pty session scrolling through multiple pages
- [x] 9.4 Fix dropped keys when a fast burst of plain characters arrives coalesced in one KeyMsg (bubbletea delivers multi-rune keys); process each rune as its own keypress; verify with unit tests for navigation bursts and burst-typed prompts

## 10. Manual Testing Feedback (Round 3)

- [x] 10.1 Render the header band at full terminal width on both lines (primary-color background behind the whole line, not just the text); verify via unit test and pty capture column measurement
- [x] 10.2 Make the list viewport follow the cursor: scroll down when the cursor leaves the bottom edge, scroll up when it returns above the top edge; the cursor row must always be visible (also fixes clampCursor silently no-op'ing due to a value receiver); verify with unit tests at multiple cursor positions
- [x] 10.3 Display search results in the exact order returned by the registry: no local sorting applied, later pages appended after earlier ones; hide the local sort key from the status line while a search is active; verify against the live registry ranking via pty capture

## 11. Theme Decision

- [x] 11.1 Preview the header band in candidate primary colors (28/34/23) via a temporary in-app switcher, then pin xterm 28 (#005F00) with white (231) text and remove the switcher; verify only color 28 renders and the T key no longer acts

## 12. Manual Testing Feedback (Round 4)

- [x] 12.1 Change the plan confirmation key from y to g so that g,g (open plan, apply) is a quick shortcut; update hints, help screen, unit tests and the e2e script
- [x] 12.2 Replace the raw-passthrough execution (suspended interface, output straight to the terminal) with a full-screen apply progress view: captured raw npm output per batch (command line + combined stdout/stderr, auto-scrolled), a what-to-do-next bottom line at every phase (step i of n / re-reading / enter-continue-q-quit), ctrl+c aborts the running invocation, all other keys ignored; verify with unit tests and a pty session against a throwaway prefix
