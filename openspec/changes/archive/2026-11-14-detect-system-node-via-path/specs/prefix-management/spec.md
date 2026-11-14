## ADDED Requirements

### Requirement: System node discovered via PATH scan
In addition to version-manager directories, the system SHALL discover system/OS Node installations by scanning each directory on the machine's `PATH` for a Node binary (`node` or `nodejs`). Any such binary whose resolved prefix is not already represented by a detected nvm/fnm/volta directory SHALL be surfaced as an environment with source label `system`. Discovered prefixes SHALL be deduplicated by resolved real path so that symlinked or aliased installs are not listed more than once. The active npm prefix reported by the local npm configuration SHALL continue to determine which environment is marked active; PATH scanning SHALL NOT change default prefix selection.

#### Scenario: System node on PATH outside version managers
- **WHEN** a `node` binary exists at `/usr/bin/node` and no nvm/fnm/volta directory contains that prefix
- **THEN** the prefix switcher lists it as a `system` environment alongside any version-manager environments

#### Scenario: Node inside a version-manager directory is not duplicated
- **WHEN** `PATH` includes an nvm version directory (e.g. `~/.nvm/versions/node/v24/bin`) that is already discovered by the nvm scan
- **THEN** that prefix appears exactly once (from the nvm scan) and is not added again as a separate `system` environment

#### Scenario: Symlinked installs are deduplicated
- **WHEN** two `PATH` entries resolve to the same real Node prefix
- **THEN** the switcher lists that prefix only once

#### Scenario: Active-prefix default selection unchanged
- **WHEN** NVM is active so `npm config get prefix` points inside an nvm directory, and a system node also exists on `PATH`
- **THEN** the initially selected environment is still the active npm prefix (the nvm one), and the system node is additionally available to select
