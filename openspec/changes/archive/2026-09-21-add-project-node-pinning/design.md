# Design: add-project-node-pinning

## Context

Project mode (`NewProject(root)`) currently resolves its toolchain lazily: every project path (ListInstalled, registry, Execute) calls `prefix.Active(ctx)` — the shell's active npm prefix. The seam from `establish-ecosystem-seam` keeps all Node-ecosystem knowledge (prefix detection, layout assumptions) inside `internal/adapter/npm` + `internal/prefix` + `internal/npmcmd`. The recent toolchain hardening (`NPMCommand`) already validates the standard layout `<prefix>/bin/node` + `<prefix>/lib/node_modules/npm/bin/npm-cli.js` and falls back to PATH npm for project-scoped runs, so any resolvable prefix is safe to bind.

## Goals / Non-Goals

**Goals:**
- Resolve a `.nvmrc` pin once at project startup and bind the matching installed prefix as the project toolchain.
- Keep the resolution pure and unit-testable (pin string × installed versions → chosen version).
- Degrade to today's behavior (active prefix) with a non-blocking notice when the pin is absent, unparseable, or unmatched.

**Non-Goals:**
- No auto-install of missing Node versions (no nvm install integration).
- No support for version-manager aliases (`lts/*`, `node`, `system`) — unparseable pins are ignored per spec.
- No `.nvmrc` handling in global mode; no status-line change (80x24 budget preserved).
- No pinning for non-Node adapters (composer/gomod have no equivalent here).

## Decisions

**D1: Resolution lives in the npm adapter at construction, not in the app.**
`NewProject(root)` becomes `NewProject(ctx context.Context, root string)`. It reads `<root>/.nvmrc`, runs `prefix.Detect` once, matches the pin, and stores the result (`projectPrefix`) plus an optional notice on the instance. Rationale: `.nvmrc` semantics and installed-prefix matching are Node-ecosystem knowledge that the seam keeps out of `internal/app`; the app only surfaces the returned notice. Alternative considered — app resolves and passes a prefixID into the adapter: rejected, it would leak Node layout/version logic into the TUI layer and duplicate detection.

**D2: Pure matching function in `internal/prefix`.**
`MatchPin(pin string, versions []string) string` — trims whitespace, strips one leading `v`, accepts 1–3 dot-separated numeric components; full pin → exact match only, partial pin → highest installed version with the pin as a dot-component prefix; returns `""` when unparseable or unmatched. Matching runs over all detected prefixes (nvm/fnm/volta and the active system prefix). Rationale: one small pure function is trivially unit-testable and reusable; including the system prefix means a pin matching the machine's system node works too, and `NPMCommand`'s layout check already guards non-standard system layouts. Alternative — match only version-manager prefixes: rejected as an arbitrary exclusion with no user-visible benefit.

**D3: Project paths use the bound prefix; fallback is the active prefix.**
ListInstalled/registry/Execute read `e.projectPrefix` instead of calling `prefix.Active(ctx)` per operation. When no pin applies, `projectPrefix` is set to `Active()` at construction (identical behavior to today, minus one exec per operation). The notice string is exposed via a getter for the app's startup notices.

**D4: Notice-only visibility.**
A missing pinned version produces a single non-blocking startup notice ("Node 24 pinned in .nvmrc not installed; using active toolchain"). No status-line column, no new screen. Rationale: the information is one-shot and diagnostic; the status line is already at its 80x24 budget.

## Risks / Trade-offs

- [Pin matches a prefix whose node binary is broken] → same failure mode as today's active prefix (per-environment load-failure notice); no new risk class, and `NPMCommand`'s PATH fallback covers layout mismatches.
- [`prefix.Detect` at construction execs `node -p` per installed version] → bounded by the number of installed versions (9 on the reference machine), once per startup, parallel to what global mode already does in Discover.
- [`.nvmrc` with an alias (`lts/*`) is silently ignored] → spec'd as "unparseable → no pin, no notice"; users who rely on aliases keep today's behavior rather than getting a wrong toolchain.

## Migration Plan

No migration: additive behavior in project mode only; global mode and all existing flows are untouched. Rollback = revert the change (the active-prefix path remains the fallback inside the same code).
