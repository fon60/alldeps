## 1. Versions-screen +/- handlers

- [x] 1.1 Add `+` to `updateVersions`: set an install/upgrade mark at the cursor version (TargetVersion) on eligible environment(s), toggling off on repeat, and never issuing a removal; verify a unit test shows `+` on a not-installed package sets MarkInstall{v}, a repeat clears it, and no removal mark is ever created.
- [x] 1.2 Add `-` to `updateVersions`: set a removal mark (toggle) only when the package is installed in an eligible environment, otherwise a no-op with a notice, and never issuing an install; verify a unit test shows `-` on an installed package sets MarkRemove and on a not-installed row does nothing while surfacing a notice.

## 2. Multi-environment popup

- [x] 2.1 Generalize `OverlayTargets` to carry a mode (install/remove) using `+` to include and `-` to exclude the cursor environment, keeping enter/space to confirm and esc/q to cancel; route `+`/`-` from the versions screen into it when more than one environment is eligible; verify a unit test shows the popup lists eligible envs, `+`/`-` toggle membership, and confirming records marks only for selected envs.
- [x] 2.2 Ensure overlay keys are intercepted before tab dispatch so `+`/`-` inside the popup do not also reach the versions screen; verify a unit test simulating a key while the overlay is open affects only the overlay.

## 3. Pin removal and verification

- [x] 3.1 Retire the enter/space pin: stop routing enter/space to a mark in `updateVersions` (remove/repurpose `pinVersion`) so only `+`/`-` mark; verify no key routes enter/space to a mark and the old pin test is removed or updated.
- [x] 3.2 Run `go test ./internal/app/` and a pty flow: on the versions screen, `+` pins an install for one env, `-` marks removal, and with more than one env the popup appears and drives per-env marks; verify tests are green and the pty flow behaves as specified.
