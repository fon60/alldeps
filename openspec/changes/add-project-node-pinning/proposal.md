# Proposal: add-project-node-pinning

## Why

In project mode npmitude runs the project's npm commands with whatever Node/npm prefix is active in the launching shell. Projects commonly pin their expected Node version in `.nvmrc` (e.g. `vtc` pins `22`); when the shell's active version differs, operations run against the wrong toolchain — confusing results and a likely source of user suspicion after the `/usr` prefix incident.

## What Changes

- Project mode reads `.nvmrc` from the project root at startup.
- A pin that matches an installed Node prefix (nvm/fnm/volta) selects that prefix's node + npm as the project toolchain: full versions match exactly, partial versions (`22`, `22.15`) match the highest installed version with that prefix.
- If the pinned version is not installed, the active prefix's toolchain is used (today's behavior) and a non-blocking notice is shown; operations are never blocked by a missing pin.
- A missing or unparseable `.nvmrc` leaves behavior unchanged (active prefix, no notice).
- Global mode is unaffected.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `project-scope`: adds the Node toolchain pinning requirement (capability introduced by the in-flight `add-project-scope-mode` change; apply after it or alongside — the delta only adds requirements).

## Impact

- `internal/adapter/npm` — project startup resolves and binds the pinned prefix; project paths (ListInstalled, registry, Execute) use it instead of the bare active prefix.
- `internal/prefix` (or a small helper beside it) — pure version-matching function for pins vs installed versions.
- `internal/app` — surfaces the fallback notice at project startup.
- No new dependencies; no protocol changes; global mode and all other capabilities untouched.
