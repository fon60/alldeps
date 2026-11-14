## Why

When a version manager (nvm/fnm/volta) owns the shell, `npm config get prefix` resolves to a directory inside that manager, so the machine's system/OS Node install is never surfaced as an environment. Users cannot see or operate on their system node while a VM is active.

## What Changes

- Add a PATH-scan discovery source: walk each directory in `$PATH` for a `node` binary; any whose resolved prefix is not already an nvm/fnm/volta directory becomes a `system` environment.
- Dedup discovered prefixes by resolved real path so symlinked or aliased installs are not duplicated.
- `npm config get prefix` continues to determine which environment is `Active`, so default prefix selection is unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `prefix-management`: add a requirement that system/OS Node installations are discovered by scanning `$PATH` for node binaries outside version-manager directories, with dedup and unchanged active-prefix semantics.

## Impact

- `internal/prefix/detect.go` (new PATH scan + integration into `Detect`) and `detect_test.go`.
- `internal/adapter/npm` Discover mapping flows the new system envs through unchanged (the `Environment.Meta` source label already supports `"system"`).
- No port/type changes; no effect on locks, marks, or planning.
