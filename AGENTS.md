# AGENTS.md

## Project Overview

`alldeps` is a terminal UI (aptitude-style) for managing globally installed packages across multiple ecosystems: Node (npm/yarn/pnpm, with nvm/fnm/volta/system prefixes), PHP (composer) and Go modules — with more package managers (pacman, yum, apt) planned. Written in Go with the Charm ecosystem. The spec lives in `openspec/` — read the active change's `proposal.md`, `design.md`, and `tasks.md` before implementing.

## Setup Commands

- Build: `./build.sh` → static binary at repo root `./alldeps` (CGO_ENABLED=0, -trimpath, stripped)
- Release (local, from a tag): `goreleaser release --clean --skip-publish` → artifacts in `dist/`, then `gh release create v<ver> --title "alldeps v<ver>" --notes-file dist/alldeps_<ver>_CHANGELOG.md dist/*.tar.gz dist/SHA256SUMS`. Test first with `goreleaser release --clean --skip-publish --snapshot` (no tag needed, nothing published). Version is injected from the tag via `.goreleaser.yml` ldflags.
- Test all: `go test ./...`
- Test one package: `go test ./internal/domain/`
- Run: `./alldeps` (rebuild with `./build.sh` first — a stale binary causes confusing "changes not visible" bugs)
- Toolchain: Go 1.27.1; deps pinned in go.mod (bubbletea v1.3.10, lipgloss v1.1.0, bubbles v1.0.0)

## Project Structure

```
main.go                 entry point (tea.NewProgram(app.New()))
build.sh                build script
test/
  e2e-session.sh        end-to-end pty session test (search → mark → plan → apply), self-contained throwaway prefix
internal/
  app/                  Bubble Tea model: model.go (state+cmds), update.go (key routing), view.go (rendering)
                        screens: list, picker, plan, info, versions, readme; quit-confirm + hint bar
  domain/               AppState / PrefixState / PkgState, marks, sorting (CompareVersions, SortRows, SortVersions), plan invariant; imports nothing under adapter/ or TUI pkgs
  ecosystem/            Ecosystem port (Discover, ListInstalled, Search, Resolve, Execute, Lock, DetectProject, Capabilities) + Intent/Plan/Conflict/ResolutionOption/Environment/Hit/Caps types
  adapter/              npm/ (npm+yarn+pnpm variants), composer/, gomod/ — concrete Ecosystem impls; npm/ delegates to npmcmd/registry/prefix/sizes and owns op-verb + layout strings
  npmcmd/               runs `npm ls -g --all --json` per prefix, parses tree, GetRegistry, LocalDoc, Readme
  registry/             HTTP client: dist-tags, outdated checks (TTL cache), search, full Doc (GetDoc)
  filter/               filter expression parser (~i ~u ~b ~n <regex>, ! & |, parens)
  prefix/               prefix detection (nvm/fnm/volta + active npm prefix) and Active()
  sizes/                background disk-size measurement (hardlink-deduped)
  lock/                 session-scoped exclusive per-environment lock ($XDG_STATE_HOME/alldeps/locks)
openspec/               specs + changes (active change: openspec/changes/<name>/tasks.md)
```

Tests are co-located (`_test.go` in the same package/directory). The root `package.json` exists only for the OpenSpec CLI — it is not part of the Go app.

## TUI Verification Pattern

The app is interactive; verify behavior through a pty with piped keystrokes:

```bash
./build.sh >/dev/null
{ sleep 3; printf 'q'; sleep 0.5; } | script -qec "stty cols 80 rows 24; ./alldeps" /dev/null > ./tmp/out.txt 2>&1
sed 's/\x1b\[[0-9;?]*[a-zA-Z]//g' ./tmp/out.txt | tr -d '\r' | grep -vE '^ *$'
```

- Target terminal size is **80x24** — all layouts must fit it (status line format: `%d/%d packages, %d pending  sort:%s  f:%s  <tilde-path>`).
- bubbletea diff-renders frames, so pty captures are fragmentary; prefer deterministic Go unit tests for logic, use pty runs only for visual/flow checks.
- Simulate offline: `env npm_config_registry=http://127.0.0.1:9/ ./alldeps`.
- PHP-version-dependent tests (composer platform conflicts) run in docker against a pinned image, not the host PHP — see `test/e2e-composer-docker.sh` (`php:8.1-cli`); the solver's platform verdict must be reproducible from the image tag.
- Test fixtures for npm JSON live in `internal/npmcmd/*_test.go`; registry behavior is tested against local `httptest` stubs (no real network in unit tests).

## Code Style

- Plain Go, no code comments unless the why is non-obvious.
- No new dependencies without need; the Charm trio (bubbletea/lipgloss/bubbles) is the only TUI stack.
- lipgloss v1.1.0 has **no** `lipgloss.Truncate` — use the local `fitText` helper in `internal/app/view.go`.
- Keep rendering pure: `View()` derives everything from `Model`; all mutation happens in `Update()`.

## Critical Gotchas (learned the hard way)

- `Model.Update` has a **value receiver**: unit tests MUST reassign the returned model after every `Update` call (see the `m.step(t, msg)` helper pattern in `internal/app/*_test.go`). Mutations via the shared `*domain.AppState` pointer are visible regardless; Model field changes (prompt/screen/cursor) are not.
- Prefix-scoped npm invocation: `<prefix>/bin/node <prefix>/lib/node_modules/npm/bin/npm-cli.js ls -g --all --json`. npm resolves `execPath` symlinks, so real prefix binaries work even when called directly.
- Broken/missing dependencies require `--all`; the command exits 1 (ELSPROBLEMS) but stdout is still clean JSON — parse stdout regardless of exit code.
- Size measurement must dedup hardlinks by (dev, ino) to match `du` (apparent size should equal `du -sb` exactly).
- Registry search API: the response `time` field is a **string**, not an int; scoped package names are URL-encoded as `%40scope%2Fname`.
- A fast burst of plain characters can arrive coalesced into a single `tea.KeyMsg` with multiple runes (`msg.Runes`). Key routing MUST process each rune as its own keypress (see the multi-rune branch in `Model.Update`); matching on `msg.String()` alone silently drops the whole burst.

## Do

- Follow the OpenSpec workflow for feature work: use the `openspec-*` skills (see `.opencode/skills/`) and flip `- [ ]` → `- [x]` in the active change's `tasks.md` only after a task is implemented AND verified.
- Keep unit tests hermetic (fixtures + httptest stubs); real-environment checks are separate and explicitly named (e.g. `TestLSGlobalRealPrefix`).
- Verify against this machine's real nvm layout (`~/.nvm/versions/node/`, 9 versions) when a task says "manual check".
- Use `./tmp/` for all scratch files, pty captures and throwaway fixtures — never system `/tmp`.

## Don't

- Don't run the app or tests expecting the previous binary — rebuild with `./build.sh` first.
- Don't add TUI libraries beyond the Charm trio.
- Don't let registry/network failures block first paint or mutate list state (search/outdated failures must be non-fatal notices).
- Don't commit the built `./alldeps` binary or `node_modules/`.
