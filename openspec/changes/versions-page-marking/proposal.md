## Why

The versions screen only supports pinning a version with enter/space on the active environment; it lacks the `+`/`-` marking available on the package list and search, and has no multi-environment selection. Users expect parity between the versions screen and the main lists.

## What Changes

- Add `+` to the versions screen: mark the version under the cursor for install at exactly that version (toggle: clears an existing install mark). It never produces a removal of an already-installed package.
- Add `-` to the versions screen: mark for removal (toggle: cancels an existing removal mark). It never produces an install.
- When more than one environment is eligible, show a scrollable environment popup where `+` includes an environment and `-` excludes/cancels it, generalizing the existing install-targets popup.
- Enforce orthogonality: `+` manages only the install mark; `-` manages only the removal mark.
- Remove the enter/space pin: on the versions screen only `+`/`-` mark; enter/space no longer perform a marking action (the user leaves with esc/q).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `package-info`: new requirement "Version-level install/remove marking" for the versions screen — `+`/`-` toggles, multi-environment popup, and orthogonality constraints (extends the existing "Version history" behavior).

## Impact

- `internal/app` (`updateVersions` key routing; reuse/adapt the `OverlayTargets` popup; mark-setting helpers in model.go).
- Depends on `automatic-dependency-flags` so the versions screen renders the 3-slot flags first.
