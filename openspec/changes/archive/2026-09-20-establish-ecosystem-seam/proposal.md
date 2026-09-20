## Why

The roadmap — yarn/pnpm next, then composer/gomod/pacman — requires a manager-agnostic core, but today npm-specific knowledge (op verbs `i -g`/`rm -g`, the `<prefix>/lib/node_modules` layout, the registry HTTP protocol, nvm/fnm/volta prefix detection) is wired directly into the app and state layers. Every future package manager would then force a refactor of shared code instead of dropping in an adapter. This change cuts that seam now, while npm is still the only manager, so all later ecosystems are additive.

## What Changes

- Introduce a dependency-free **domain core** that owns the aptitude workflow's invariants: version comparison, state/action flags, plan grouping, mark/reconcile rules, and the *one-manager-per-destination* plan invariant. It has zero imports of any adapter or TUI package.
- Define a single **`Ecosystem` adapter port**: discover environments, list installed packages, search, resolve an intent into a concrete plan (or a set of conflicts), execute a plan with streamed output, acquire/release a native-preferred lock, and detect project applicability.
- Move **all npm behavior** behind the port in `internal/adapter/npm`. The app depends only on the domain core + the port — never directly on `npmcmd`, `registry`, or `prefix`.
- Refactor identity to **(Ecosystem, Environment, Name)**: environment IDs become adapter-defined; installed state is stored **per destination** while pending marks are stored **per (destination, manager)**.
- Add an **import-boundary test**: the domain core imports nothing from any adapter; the app imports no adapter except through the port.

No user-visible behavior changes. The npm-only UI, keymap, flags, and flows behave exactly as before.

## Capabilities

### New Capabilities
None. This is a pure internal refactor with no spec-level behavior change.

### Modified Capabilities
None. `skip_specs: true` is set in `.openspec.yaml`; the adapter contract and domain model are captured in `design.md`, not as behavior specs.

## Impact

- `internal/state` → becomes the dependency-free domain core (renamed `internal/domain`); npm-specific fields (`Broken`, `NodeVersion`, `RegistryURL`) become adapter-supplied metadata.
- `internal/npmcmd`, `internal/registry`, `internal/prefix` → absorbed by `internal/adapter/npm` (they remain usable as node-ecosystem implementation details the adapter imports).
- `internal/app` → depends on `domain` + the `Ecosystem` port only; op-verb and layout strings move out of `model.go` into the npm adapter.
- New packages: `internal/domain`, `internal/ecosystem` (port interface), `internal/adapter/npm`.
- Tests: existing unit tests must pass unchanged (behavior preserved); a new import-boundary test is added; `test/e2e-session.sh` must still pass against the rebuilt binary.
