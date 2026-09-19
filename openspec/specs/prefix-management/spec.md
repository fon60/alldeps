# prefix-management Specification

## Purpose

Detect the Node installation prefixes present on the machine (version managers and system) and let the user select which prefix all npmitude operations target.

## Requirements

### Requirement: Prefix detection
On startup and on refresh, the system SHALL detect available Node prefixes from nvm, fnm, and volta installations as well as the active npm prefix reported by the local npm configuration. Each detected prefix SHALL be presented with its Node version (where determinable) and the number of globally installed packages it contains.

#### Scenario: nvm versions discovered
- **WHEN** the machine has five nvm-managed Node versions, each with global packages
- **THEN** the prefix switcher lists all five with their versions and per-prefix package counts

#### Scenario: System-only installation
- **WHEN** no version manager is present and only a system Node exists
- **THEN** the switcher lists the single active npm prefix

### Requirement: Default prefix selection
The initially selected prefix SHALL be the active npm prefix — the one plain `npm` commands would use.

#### Scenario: Launch without explicit selection
- **WHEN** the user launches npmitude without choosing a prefix
- **THEN** the list and all operations target the active npm prefix

### Requirement: Prefix switching
The user SHALL be able to switch the selected prefix at any time. After switching, the package list, upgradability state, and status line SHALL reflect the newly selected prefix. Pending marks are scoped per prefix and MUST be preserved when switching away and back.

#### Scenario: Switch between versions
- **WHEN** the user switches from Node v24 to Node v22
- **THEN** the list shows v22's globally installed packages and the status line shows v22's prefix path

#### Scenario: Marks survive a round trip
- **WHEN** the user marks two packages on v24, switches to v22, then switches back to v24
- **THEN** the two pending marks are still present on v24

### Requirement: Exclusive environment access
Opening an environment SHALL acquire an exclusive advisory lock for that environment; the lock is held for the lifetime of the open environment and released when the environment is closed, when the user switches away from it (the new environment's lock MUST be acquired before the old one is released), or when the application exits. Attempting to open an environment whose lock is held by another live instance SHALL be refused with a notice identifying the holder. A lock whose holder is no longer running SHALL be treated as stale and cleared automatically.

#### Scenario: Second instance is refused
- **WHEN** one npmitude instance has environment X open and a second instance attempts to open X
- **THEN** the second instance is refused with a notice identifying the holding instance, and X remains open in the first instance

#### Scenario: Switching without a gap
- **WHEN** the user switches from environment X to environment Y while holding X's lock
- **THEN** Y's lock is acquired before X's lock is released, so no moment exists in which neither is held

#### Scenario: Crash recovery
- **WHEN** an instance that held environment X's lock terminates abnormally and another instance later attempts to open X
- **THEN** the stale lock is detected (holder not running) and cleared, and X opens normally

### Requirement: Persistent prefix display
The currently selected prefix path SHALL be visible at all times in the status line.

#### Scenario: Status line shows prefix
- **WHEN** the user is viewing any screen of npmitude
- **THEN** the selected prefix path is shown in the status line

### Requirement: Vanished prefix handling
If a previously detected prefix no longer exists (e.g. its Node version was removed), the system SHALL drop it from the switcher on refresh; if it was the selected prefix, the system SHALL fall back to the active npm prefix and inform the user.

#### Scenario: Selected version removed
- **WHEN** the user's selected nvm version is deleted outside npmitude and npmitude refreshes
- **THEN** that prefix disappears from the switcher, selection falls back to the active npm prefix, and a notice is shown
