# Design: add-composer-adapter

## Context

The port, project-mode wiring, and the full conflict flow (list marking, resolver screen, plan gate, apply-subset execution, background re-resolve) already exist from `establish-ecosystem-seam` and `add-conflict-resolution`; no adapter today produces resolver-reported conflicts, so composer is their first real consumer. Composer specifics that shape this design: `vendor/composer/installed.json` records exact installed versions (better truth than npm's `ls`); Packagist provides search + version metadata but no unpacked sizes; the solver genuinely fails with structured "Problem N:" blocks; `--dry-run` runs the full solver without touching the project; lifecycle scripts are a normal part of any composer run.

## Goals / Non-Goals

**Goals:**
- A `composer` adapter: listing, Packagist search/outdated, require/remove/update execution with scripts enabled.
- Dry-run-based conflict detection feeding the existing resolver flow with concrete, machine-applicable options.

**Non-Goals:**
- COMPOSER_HOME "global" scope (niche; global mode stays npm-only).
- Executable "upgrade PHP" options (platform facts are shown in consequences only).
- Size deltas on options (Packagist has no unpacked sizes — the spec's "where applicable" covers this).
- Re-implementing the solver (impossible); we parse its *output*, not its logic.

## Decisions

**D1: `vendor/composer/installed.json` is the listing truth.**
ListInstalled reads installed.json and shows packages whose names are keys of `composer.json`'s `require` (direct dependencies), with exact installed versions. Alternative considered — `composer show`: rejected, it needs the binary for a read-only view and its output is prose-shaped; the file is precise, hermetic, and needs no toolchain. No vendor dir → empty list (operations still work; composer will create vendor on first require).

**D2: `--dry-run` is the conflict probe.**
Resolve (and a background re-probe) runs each pending op kind through `composer <op> ... --dry-run --no-interaction` in the project dir. Exit 0 → normal plan batches, no conflicts. Non-zero with "Your requirements could not be resolved..." → parse the Problem blocks into Conflicts. Probing happens when marks change (debounced, background) and at plan open, so list markers and the gate are already populated. Rationale: it is the only way to know the solver's verdict without re-implementing it, and dry-run guarantees no side effects. Alternative considered — probe only at apply time: rejected, conflicts would first appear as apply failures instead of as resolvable plan-gate items.

**D3: Problem classification with a guaranteed fallback.**
The parser recognizes a small set of problem classes and derives options from solver output plus Packagist metadata; anything unrecognized yields a fallback Conflict carrying the raw solver text (Message) with generic options. Every option carries a machine-applicable `ResolutionEffect` (pin version / remove dependency / skip), so the existing "apply chosen option → update marks → re-resolve" loop works unchanged:

| Problem class | Recognized from | Derived options (effects) |
|---|---|---|
| Platform mismatch | "requires php ^X (your php ... does not satisfy)" | pin target's last release line supporting the platform version (from Packagist metadata; consequence states the requirement), skip |
| Root-dep clash | two root requirements whose constraints intersect unsatisfiably ("satisfiable by ..." candidate lists) | pin A@v / pin B@v (candidates named in the output), remove one of them, skip |
| Package not found | "package X could not be found" | corrected-name suggestion when Packagist search returns a close match, skip |
| Unrecognized | anything else | fallback: raw text + choose-version / skip |

The parser is best-effort by design: composer rewords solver output across versions, and the fallback guarantees the flow never dead-ends.

**D4: Scripts enabled, interactivity suppressed.**
Execute runs `composer require <name>:<constraint> --no-interaction` / `composer remove <name> --no-interaction` in the project dir with lifecycle scripts running normally (a user who marked a package expects the project's own post-install behavior; disabling scripts would silently skip asset builds). Toolchain: `composer` from PATH; the platform PHP version (`php -r 'echo PHP_VERSION;'`) is detected once to enrich conflict consequences ("your php 8.1.27"). Missing composer/php → clear load-failure notice, same class as npm's missing prefix.

**D5: ProjectScope only, session lock.**
`Caps`: GlobalScope=false (COMPOSER_HOME out of scope), ProjectScope=true, HasSearch=true, HasConflictResolution=true, HasNativeLock=false (`composer.lock` is data, not a mutex — the session-scoped lock applies as for npm), HasDedupe=false (vendor is per-project; no cross-destination redundancy). Discover returns the single project environment (project root, kind `composer-project`).

## Risks / Trade-offs

- [Solver output wording drift across composer versions] → best-effort classification + guaranteed fallback Conflict (D3); a regression degrades to "raw text + generic options", never to a blocked flow.
- [Dry-run cost: a full solver run per probe] → debounce on mark changes, cache results keyed by the pending-mark set, and reuse the plan-open probe for the gate.
- [Packagist metadata gaps (no sizes, dev tags)] → latest-stable preference skips `-dev`/`-alpha` releases; options carry nil SizeDelta.
- [Scripts running arbitrary project code during apply] → accepted by decision (D4); raw output is streamed in the apply screen so the user sees exactly what ran.
- [Vendor dir absent or stale vs composer.json] → listing reflects installed.json only; a stale vendor shows outdated "installed" state, corrected after any apply's post-run re-list.

## Migration Plan

No migration: additive adapter behind existing capability flags; global mode and npm behavior untouched. Rollback = revert the change.
