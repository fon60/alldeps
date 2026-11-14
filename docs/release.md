---
name: release
description: Cut and publish a new alldeps release — tag, goreleaser build, GitHub release upload, verification. Use when the user wants to release, tag, or publish a new version of alldeps.
compatibility: Requires goreleaser and gh on PATH (both in ~/go/bin, source ~/.profile) and an authenticated gh session for github.com with repo scope.
metadata:
  author: fon60
  version: "1.0"
---

# Release procedure

Publish a tagged release of alldeps: cross-compiled binaries (linux/darwin × amd64/arm64) + SHA256SUMS + changelog to GitHub Releases. Everything builds locally with goreleaser and uploads with gh — no CI, no goreleaser publish pipeline.

**Prerequisites**

1. `source ~/.profile` — goreleaser and gh live in `~/go/bin`
2. `gh auth status` → must show logged in to github.com with `repo` scope. If not: `gh auth login --hostname github.com --git-protocol https --web` (browser device flow — relay the one-time code to the user and wait for them to authorize)
3. Clean working tree — goreleaser's validation fails on ANY dirty state, including untracked files

**Steps**

1. **Prepare the tree**

   Commit or stash all pending changes (`git status`). Local artifacts that must never be committed belong in `.gitignore` (currently: `dist/`, `skills-lock.json`) — ignore them, don't commit them.

2. **Vulnerability scan (mandatory, before the snapshot dry-run)**

   ```bash
   ./vuln.sh
   ```

   govulncheck in source mode: reachability-based CVE scan of the module graph and stdlib. "No vulnerabilities found" → continue. A known reachable vulnerability blocks the release until the vulnerable dependency is updated (re-run `./vuln.sh` to confirm clean) or the risk is explicitly accepted by the user — record any such acceptance in the release notes.

3. **Snapshot dry-run (always, before tagging)**

   ```bash
   goreleaser release --clean --snapshot
   ```

   No tag needed, nothing published. Verify:
   - `ls dist/` → 4 tarballs + `SHA256SUMS`
   - Execute the linux/amd64 binary in a pty and confirm the header line shows the snapshot version — this proves ldflags version injection works:
     ```bash
     cd dist/alldeps_linux_amd64_v1 && { sleep 3; printf 'q'; sleep 0.5; } | script -qec "stty cols 80 rows 24; ./alldeps" /dev/null
     ```
   - Cross-compile errors (e.g. darwin `syscall.Stat_t` field widths) only surface here — fix them and re-run before tagging.

4. **Tag**

   ```bash
   git tag v<ver>        # semver with v prefix, e.g. v0.2.0
   ```

   If the tree changes after tagging, re-point: `git tag -f v<ver>`. Do NOT hand-edit the `Version` var in `internal/app/model.go` — it is only the dev-build fallback; the release version is injected from the tag via `.goreleaser.yml` ldflags.

5. **Build release artifacts**

   ```bash
   goreleaser release --clean --skip=publish
   ```

   Produces `dist/alldeps_<ver>_{linux,darwin}_{amd64,arm64}.tar.gz`, `dist/SHA256SUMS` and the changelog (check `ls dist/` for its exact name — currently `dist/CHANGELOG.md`).

6. **Push branch + tag**

   ```bash
   git push origin main v<ver>
   ```

   `gh release create` refuses to run until the tag exists on the remote.

7. **Create the GitHub release**

   ```bash
   gh release create v<ver> --title "alldeps v<ver>" \
     --notes-file dist/CHANGELOG.md \
     dist/alldeps_<ver>_linux_amd64.tar.gz dist/alldeps_<ver>_linux_arm64.tar.gz \
     dist/alldeps_<ver>_darwin_amd64.tar.gz dist/alldeps_<ver>_darwin_arm64.tar.gz \
     dist/SHA256SUMS
   ```

8. **Verify**

   ```bash
   gh release view v<ver> --json assets       # expect 5 assets
   go list -m github.com/fon60/alldeps@latest # must resolve to v<ver> (unlocks go install)
   ```

9. **Post-release housekeeping (only if the version in docs went stale)**

   The README Install section pins a concrete download URL (currently v0.1.0). Bump it to the new version and commit.

**Gotchas (learned cutting v0.1.0)**

- The flag is `--skip=publish`, not `--skip-publish` (goreleaser v2)
- The changelog file name is goreleaser-version-dependent — always `ls dist/` instead of assuming
- Dirty-tree validation counts untracked files; a stray lockfile will block the build
- `main` has branch protection ("Changes must be made through a pull request"); direct pushes bypass it via admin rights. If push output shows "Bypassed rule violations", flag it to the user
