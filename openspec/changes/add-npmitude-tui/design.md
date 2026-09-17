# Design: add-npmitude-tui

## Context

Greenfield repository (only OpenSpec scaffolding and an empty `package.json`). See proposal.md for motivation. Hard constraints that shape the approach:

- Each managed prefix contains its own Node + npm installation, and writes to the global tree MUST go through that prefix's npm so lifecycle scripts, peer handling, and permission behavior stay exactly as the user's npm would do them. npmitude itself is a self-contained binary with no host runtime requirement.
- There is no local package universe in npm (unlike apt's lists): discovery is on-demand registry search only.
- The target machine may run several Node versions via nvm/fnm/volta, each with its own global prefix; npmitude must operate on any of them, including ones different from the host's active npm.
- Network access to the registry can fail at any time; the UI must stay usable offline (local data only).

## Goals / Non-Goals

**Goals:**
- aptitude-feel UX: flag-column list, mark → plan preview → apply, filter expressions, info screen, prefix switcher.
- Clean tool/host boundary: npmitude runs as a static binary independent of the host Node version; npm is a managed target, never a runtime dependency of the tool.
- Local-first startup: the installed list renders before any network response arrives; all remote enrichment (latest versions, search) is progressive.
- One coherent state model keyed by `(prefix, package)` so multi-prefix is a data property, not a special case.
- Degrade gracefully: every network failure yields a notice, never a broken or cleared UI.

**Non-Goals:**
- Maintaining a local index of the registry universe (explicitly rejected — on-demand search only).
- pnpm/yarn support and project-level (non-global) scope — deferred extensions; the environment abstraction (D4) is designed so these become additive changes, not refactors.
- Installing/removing Node versions themselves; npmitude lists prefixes but does not mutate them.
- Editing npm configuration or managing auth tokens.
- Full aptitude keymap/format/grouping parity in v1 (the v1 set is fixed in D8).
- Package grouping by category, aptitude-style version pinning — candidates for later changes. (README viewing is in v1 — D9.)
- Persisting any state between runs (last-selected prefix first) — the designated follow-up after v1; see D6.

## Decisions

### D1: TUI framework — Go + Bubble Tea

npmitude is written in Go; the UI uses Bubble Tea (Elm architecture: Model/Update/View) with lipgloss for styled rendering and bubbles for list/text-input components. The mark → plan → apply flow maps naturally onto a single state machine, and aptitude's custom-drawn flag grid renders cleanly through lipgloss styles.

Deciding factors:
- **Self-contained binary, clean tool/host boundary** — no host runtime to install or version-match; static cross-compiled binaries per platform (GitHub releases).
- **Host Node independence** — a Node-written frontend would exclude users whose host Node is older than the tool's requirement; Go has no such constraint. The npm dependency remains by design (writes go through npm), but the tool itself never constrains it.
- **Roadmap fit** — planned pnpm/yarn and project-scope support make npmitude a manager-agnostic frontend; building it outside the JS ecosystem makes that boundary explicit.

Alternatives considered:
- *Ink + React/TypeScript* — first-class npm-ecosystem alignment (the original choice); loses on host Node independence, binary distribution, and tool/host boundary.
- *blessed-contrib / raw ANSI (Node)* — same runtime constraint, weaker fit.
- *tview (Go)* — immediate-mode widget kit with less momentum than Bubble Tea; the Elm architecture fits this stateful UI better.

### D2: Read path — direct registry HTTP + one `npm ls` per prefix load

Two sources of truth for reads:

1. **Installed state**: run `npm ls -g --json` once per prefix load (using that prefix's own npm, see D3). It yields names, versions, and tree validity in one call — invalid/missing dependencies become the `b` broken flag. Disk sizes are not in its output, so they come from a background walk of the prefix's global module directory (sizes render progressively as "…" until measured).
2. **Remote metadata**: Go's stdlib `net/http` against the configured registry (base URL read once from `npm config get registry`, refreshed on prefix switch): the search endpoint (`/-v1/search?text=…&size=N`) for discovery, package documents for descriptions/versions/dist-tags. Outdated checks are per-package `latest` dist-tag lookups, run in parallel with a concurrency limit and cached in memory with a TTL; a manual refresh key re-checks (aptitude's `u`).

- *Alternative: spawn `npm search` / `npm view --json`* — rejected: full node+npm startup per query, brittle output parsing, no concurrency control.
- Respecting the configured registry URL keeps private registries and mirrors working without extra configuration.

### D3: Write path — spawn the target prefix's own npm, raw passthrough

Apply runs `<prefix>/bin/node <prefix>/lib/node_modules/npm/bin/npm-cli.js install|uninstall -g …` (i.e. the selected prefix's own node + npm), so version-manager semantics are honored exactly even when operating on a non-active prefix, avoiding the historical quirks of `npm --prefix`.

**Version-adaptive adapter.** Each environment's npm major version is detected at load (`npm --version` via that prefix's node) and selects an adapter for behaviors that drift between npm versions (output shapes, flag availability). The rule that keeps the surface small: ask npm itself for config questions (`npm config get registry`) rather than re-implementing `.npmrc` resolution in Go — only stable JSON outputs (`npm ls -g --json`) are parsed, never prose. This same adapter interface is where pnpm/yarn will slot in later (different binaries, different JSON shapes, same seam).

**Raw passthrough run view.** On an approved plan the TUI is suspended and a status line naming the run (e.g. `Running npm install -g pnpm…`) is printed; each npm invocation's output then streams directly to the terminal, unfiltered and unparsed — no progress region, no spinner, no output parsing (the simplest thing, and it stays correct for whatever package managers we add later). The plan compiles to at most three batched invocations grouped by operation kind (`npm rm -g …`, `npm i -g …`, upgrades as `npm i -g pkg@latest`), each announced with a separator line naming the command. If an invocation exits non-zero, remaining invocations stop and the user reads npm's diagnostics in place. Ctrl-C kills the current child, aborts the rest of the plan, and is treated as the failure path.

When the run finishes (success, failure, or interruption), npmitude prints a completion prompt modeled on aptitude's: `Press Enter to return to npmitude, or q+Enter to quit.` Enter triggers the post-apply disk re-read and returns to the list; q+Enter exits.

- Single-flight: an `applying` state blocks concurrent applies within one instance (the cross-instance case is covered by the environment lock, D7).

### D4: State model — one state machine, everything keyed by environment

```
AppState {
  prefixes: Map<prefixId, PrefixState>   // prefixId = absolute prefix path
    PrefixState {
      packages: Map<name, PkgState>
      loaded: bool, sizesKnown: bool
    }
  activePrefixId
  filterExpr, sortKey
  applying: null | { ops, child }
}

PkgState {
  name
  installedVersion?   // set when present in prefix tree
  latestVersion?      // from registry, TTL-cached
  sizeBytes?          // background-measured
  broken: bool        // from npm ls validity
  origin: 'installed' | 'search'
  mark: none | install | remove | upgrade | hold
  targetVersion?      // pinned via version screen
}
```

A single Bubble Tea Model/Update pair drives the UI; View is a pure function of state. Marks live in `PkgState.mark`, so per-prefix mark preservation across switches (spec requirement) falls out for free. Search results insert/merge rows by name: an existing installed row is never duplicated; a new row gets `origin: 'search'`.

The keying concept is deliberately an *environment* — v1 has exactly one kind (an npm global prefix, identified by its path). The planned pnpm/yarn and project-scope environments (Non-Goals) reuse this shape, so they become additive changes rather than refactors.

### D5: Filter language — aptitude subset, tiny parser

Support `~i`, `~u`, `~b`, `~n <pattern>`, `!`, `&`, `|` with a small recursive-descent parser (~100 lines). Full aptitude syntax (`~v`, sections, etc.) is YAGNI and mostly unmappable to npm. Invalid expressions are rejected at parse time; the previous filter stays active (per spec).

### D6: No config file in v1

Defaults only: prefix = active npm prefix on launch (per spec), registry URL from npm config, in-memory caches lost on exit. Last-prefix persistence is the designated first follow-up after v1; page size and keybinding configuration remain out of scope.

### D7: Exclusive environment access — session-scoped lock

Only one npmitude instance may have a given environment open at a time. The lock is acquired when an environment is opened and held for its entire open lifetime; it is released on close, on exit, and on switch — where the new environment's lock MUST be acquired before the old one is released (no gap).

- **Lockfile**: `$XDG_STATE_HOME/npmitude/locks/<env-id>.lock` (default `~/.local/state/npmitude/locks/`; `<env-id>` = short hash of the absolute prefix path). One directory for all app state on Linux and macOS alike, no platform branching; kept outside the prefix so npm's territory is never touched.
- **Contents**: human-readable prefix path + holder PID + process start time + hostname — used for the "who's holding it" notice.
- **Refuse, don't wait**: opening an env held by a live instance shows a notice identifying the holder; no polling or timeout machinery in v1.
- **Stale recovery**: on open attempt, a lock whose holder PID is dead (verified against start time to dodge PID reuse) is cleared immediately; an age timeout is only a backstop. A `kill -9` crash therefore never wedges an environment in practice.
- **Startup with a locked default**: launch tries to lock the active npm prefix; if held, npmitude opens the environment picker with that env marked locked/unselectable instead of refusing to start.

### D8: v1 keymap — aptitude core set with two deliberate deviations

| Keys | Action |
|---|---|
| arrows / `j` `k` | navigate list |
| `+` `-` `=` `:` | mark install / remove / hold / revert |
| `U` `x` | mark all upgradable (respects holds) / clear all marks |
| `/` | registry search prompt — submitted on Enter, one network query per search |
| `l` | local live match — transient, installed packages only, zero registry access; cancel restores the full list |
| `f` | persistent filter-expression prompt (`~i`, `~u`, `~b`, `~n …`, `!`, `&`, `\|`) |
| `Enter` `d` `v` `C` | info screen / dependencies section / version history / README view |
| `S` | cycle sort key |
| `u` | refresh (recheck outdated + rescan prefixes) |
| `e` `E` | cycle environments / open environment picker popup |
| `g` `Q` | apply / quit — quit always confirms ("Really quit npmitude?", mirroring aptitude's Prompt-On-Exit; this also guards unapplied marks) |

Deliberate deviations from aptitude: `/` and `l` swap roles — aptitude's `/` (local search/jump) becomes our `l` restricted to installed packages, while aptitude's `l` (limit the list) becomes our `/` registry search, because we have no local universe to limit. `e`/`E` are new (no aptitude precedent for environment switching); `C` keeps aptitude's binding but opens a README instead of a changelog. Skipped aptitude keys have no npm equivalent (auto-flag `M`/`m`, purge `_`, forbid-upgrade `F`, reinstall `L`, grouping `G`, format editing `p`/`s`, reverse-deps `r`, …).

### D9: README view — plain, local-first

`C` opens a full-screen scrollable text view of the selected package's README. Source: the local README file for installed packages, else the registry document's `readme` field (fetched on demand); absent → "no README available" notice. Rendering is deliberately plain to match aptitude's look: a minimal markdown-to-text pass (headings kept, lists and code fences indented, links shown as `text (url)`, emphasis stripped), no fancy ANSI styling. Display-only — rendering failures degrade to raw text, never an error state.

## Risks / Trade-offs

- [npm behavior differences across versions when spawning a non-active prefix's npm] → Always use the target prefix's own node+npm binaries (D3); version-adaptive adapter isolates drift (D3); add an early integration spike against a multi-version nvm layout before building on top of it.
- [.npmrc/config semantics drifting between npm versions] → Ask each environment's own npm for config questions (D3 adapter rule); never re-implement or cache config resolution across environments.
- [Silent stretches during large downloads in the run view] → Accepted: raw passthrough shows whatever npm emits (including lifecycle-script output); the plan preview shows sizes beforehand, and the post-apply disk re-read guarantees truth regardless of what npm said.
- [Registry latency/rate limits on broad queries] → Search is prompt-submitted with a page cap (not unbounded), requests are timeout-bounded, dist-tag results are TTL-cached.
- [Stale upgradability data] → TTL cache + visible manual refresh; candidate column is enrichment, never a gate for list rendering.
- [Disk-size measurement cost on large prefixes] → Measured in the background after first paint; rows show "…" until known.
- [Stale lock after a crash wedging an environment] → Holder PID liveness + start-time check clears dead locks on the next open attempt; age timeout is a backstop only (D7).
- [System prefix not writable without root] → Surface an actionable permission message per spec; no automatic sudo in v1 (security decision).

## Migration Plan

Greenfield — nothing to migrate. Delivery order doubles as the rollback story: each stage is independently runnable, so a broken later stage can be reverted without affecting earlier ones.

1. Scaffold Go module + Bubble Tea app with the three-region layout and status line.
2. State model + installed list (npm ls parse, flags, sort, filter) — fully usable offline.
3. Registry search integration.
4. Environment detection + switcher + exclusive locking (D7).
5. Mark/plan/apply with the raw-passthrough run view (D3).
6. Info screen, version history, README view (D9).
7. Keymap polish and edge-case hardening (permissions, locks, failures).

## Open Questions

None outstanding. All six questions previously listed here were resolved during design review and folded into the decisions: keymap scope → D8; search input mode → D8 (`/` submit-based registry search, `l` live local match); config file → D6 (none in v1; last-prefix persistence is the designated follow-up); cross-instance locking → D7 (session-scoped exclusive lock); progress granularity → D3 (raw passthrough, no parsing); README display → D9 (included in v1).
