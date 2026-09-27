# alldeps

An aptitude-style terminal UI for global packages across npm, pnpm, yarn, composer and Go modules. Fuzzy-search your installs, inspect versions and dependencies, resolve conflicts, apply a plan — without typing full package names. Written in Go.

## What it does

`alldeps` gives you one screen over everything installed globally on the machine:

- **Unified list** — packages from every supported ecosystem in one view, with version, install state (latest / outdated), size and dependency flags.
- **Fuzzy search & filters** — type a few characters or a filter expression (`~i ~u ~b ~n <regex>`, `!` `&` `|`, parens) instead of full package names.
- **Conflict resolution** — when a marked change would break installed dependencies, `alldeps` shows the conflict and concrete resolution options before anything runs.
- **Plan → apply** — mark packages with `+` / `-`, preview the exact plan, then apply it. Nothing executes until you confirm.
- **Per-environment locking** — one session per environment; a second instance is told who holds the lock instead of clobbering state.

## Supported ecosystems

| Ecosystem | Managers | Status |
|---|---|---|
| Node.js | npm, pnpm, yarn (nvm / fnm / volta / system prefixes) | supported |
| PHP | composer | supported |
| Go | go modules | supported |
| System | pacman, yum, apt | planned |

Run with no arguments for **global mode** (everything installed on the machine), or point it at a directory (`alldeps .`) for **project mode**, where it auto-detects which managers apply to that project.

## Build

```sh
./build.sh        # static binary at ./alldeps (CGO_ENABLED=0, stripped)
# or
go build -o alldeps .
```

Requires Go 1.27+.

## Usage

```sh
./alldeps         # global mode
./alldeps .       # project mode (current directory)
```

List-screen keys:

| Key | Action |
|---|---|
| `+` `-` `=` `:` | mark install / remove / update / unmark |
| `/` | search the registry |
| `f` | filter expression |
| `r` | resolve conflicts for the selection |
| `U` `x` | show upgradable / clear marks |
| `enter` | package info (versions, readme) |
| `S` | sort |
| `e` / `E` | switch environment / manager |
| `g` | apply the marked plan |
| `?` `q` | help / quit |

## Development

```sh
go test ./...     # unit tests (hermetic: fixtures + httptest stubs)
./build.sh        # rebuild the binary before any manual/pty check
test/e2e-session.sh   # pty end-to-end: search → mark → plan → apply
```

The spec lives in `openspec/` — see the active change's `proposal.md`, `design.md` and `tasks.md`. Agent-facing notes are in `AGENTS.md`.
