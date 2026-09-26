## Context

See proposal.md for motivation. Current state: `prefix.Detect` (internal/prefix/detect.go) scans nvm/fnm/volta directories, then calls `npm config get prefix` and labels that path `"system"`, merging it into an existing entry if the path is already present (so when NVM owns the shell, no separate system row appears). There is no probe for an OS-level node. Environments are deduped by path, ranked/sorted by Node version descending, and the `Active` flag drives default selection in the app.

## Goals / Non-Goals

**Goals:**
- Discover system/OS Node installs by scanning `$PATH`, independent of which manager owns the shell.
- Surface each distinct system prefix as a `system` environment with a resolvable Node version.
- Preserve existing dedup, ranking, and active-prefix/default-selection behavior.

**Non-Goals:**
- Detecting node installed only as a shell function/alias (not a real binary on PATH).
- Changing how the active npm prefix is determined or how default selection works.

## Decisions

**D1 — Scan for a Node binary (`node` or `nodejs`) in each `PATH` directory.**
For each directory on `PATH`, test for a regular file (or symlink to one) named `node` or `nodejs`. Rationale: `node` is the canonical name, but some distros ship `nodejs`; matching both maximizes OS coverage, which is an explicit goal of this tool.

**D2 — Derive the candidate prefix as the parent of the bin directory, validated by executing the found binary.**
Given a found binary at `<dir>/<name>` (where `<name>` is `node` or `nodejs`), the candidate prefix is `filepath.Dir(<dir>)`. The candidate is kept only if the found binary actually executes and reports a version (`<dir>/<name> -p process.versions.node`, reusing the existing `nodeVersionOf` helper pointed at the real path). Rationale: this both validates that the hit is a real Node install (not a stray script) and yields the NodeVersion needed for ranking/display, with no new mechanism.

**D3 — Dedup by resolved real path, against all already-discovered prefixes.**
Resolve each candidate prefix with `filepath.EvalSymlinks` and keep it only if its real path is not already present among the nvm/fnm/volta discoveries or previously added PATH discoveries. Rationale: a PATH entry pointing into an nvm version dir (or a symlink alias of an existing prefix) must not produce a duplicate row; this also makes "two PATH entries, one real install" collapse to one.

**D4 — Active flag and default selection are unchanged.**
`npm config get prefix` still decides which environment is `Active`; the existing merge logic marks that entry active whether it came from a VM scan or the PATH scan. Default selection (active env, else first) is untouched. Rationale: the user only asked to *see* the system node, not to change what is selected by default.

**D5 — Ranking reuses the existing version-descending sort.**
PATH-discovered prefixes carry their executed NodeVersion and participate in the same sort as VM envs (version desc, tie-break by ID). Rationale: no new ordering concept; a system node sorts among the others by version.

## Risks / Trade-offs

- [Stray `node` on PATH] A non-prefix `node` file could be mistaken for an install → D2's execute-and-report-version validation rejects anything that is not a runnable Node.
- [Multiple system nodes] Distinct real prefixes (e.g. `/usr/bin/node` v18 and `/usr/local/bin/node` v20) both appear as separate `system` envs → intended and consistent with how multiple nvm versions are listed.
- [Unreadable/looping PATH dirs] Stat or symlink errors on a PATH entry → skip that entry gracefully (nil-on-error, matching the existing `scanDirs` behavior).
- [Cost] One stat per PATH dir plus an exec per candidate prefix → negligible; bounded by PATH length and only for real candidates.

## Open Questions

None. The earlier `nodejs`-naming question is resolved in D1: match both `node` and `nodejs` to maximize OS coverage.
