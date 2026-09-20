# Proposal: add-composer-adapter

## Why

The `project-scope` capability already promises that a project with `composer.json` reports Composer as an applicable manager, but today only the npm adapter exists — PHP projects launch into "no recognized package manager". Beyond filling that gap, Composer is the first ecosystem whose dependency solver genuinely fails: its conflicts are the first real consumer of the conflict-resolution capability (resolver screen, plan gate, apply-subset gating), which today has no adapter that produces resolver-reported conflicts.

## What Changes

- New `composer` adapter implementing the Ecosystem port for Composer projects (project scope only): detection via `composer.json`, listing from `vendor/composer/installed.json` (exact installed versions), search and latest-version checks against Packagist, operations via `composer require/remove/update`.
- Conflict detection through a `--dry-run` probe: pending operations are run through Composer's solver without touching `vendor/`; solver failures are parsed into conflicts with concrete resolution options (pin a compatible version, remove a conflicting root dependency, or skip), and unrecognized solver output degrades to a fallback conflict carrying the raw solver text.
- Operations run with project lifecycle scripts enabled and `--no-interaction`.
- The composer adapter is registered in project-mode startup wiring; global mode stays npm-only (the COMPOSER_HOME "global" scope is deliberately out of scope).

## Capabilities

### New Capabilities

- `composer-package-management`: listing installed packages with exact versions, Packagist search and outdated detection, composer-based install/upgrade/remove operations with scripts enabled, and dry-run-based conflict detection with solver-derived resolution options.

### Modified Capabilities

(none — the project-scope detection requirement this change fulfills is already written, and the conflict-resolution capability is manager-agnostic by design; no spec text changes)

## Impact

- `internal/adapter/composer` — new package (adapter + Packagist client).
- `main.go` — registers the composer adapter in project mode.
- The app's existing conflict flow (list marking, resolver screen, plan gate, apply-subset, background re-resolve) is consumed as-is; no app changes expected beyond what capability flags already drive.
- No new dependencies; global mode and all other capabilities untouched.
