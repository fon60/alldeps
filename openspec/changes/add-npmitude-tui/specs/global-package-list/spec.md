## Purpose

Present the globally installed packages of the selected Node prefix in an aptitude-style list with state/action flags, versions, sizes, and upgradability against the registry.

## ADDED Requirements

### Requirement: Installed package listing
The system SHALL display every top-level package installed in the global module directory of the selected prefix, showing for each: a two-character state/action flag, package name, disk size, installed version, and candidate version when different from installed.

#### Scenario: Listing globals on launch
- **WHEN** the user launches npmitude and the selected prefix has three globally installed packages
- **THEN** the list shows exactly three rows, each with state flag `i`, the package name, its installed version, and its disk size

#### Scenario: Empty prefix
- **WHEN** the selected prefix has no globally installed packages
- **THEN** the list is empty and the status line reports zero packages

### Requirement: State and action flags
Each row SHALL display a current-state character (`i` installed, `p` available-not-installed, `b` broken) followed by a pending-action character (`*` none, `+` install, `-` remove, `u` upgrade, `h` held). The action character reflects only marks that have not yet been applied.

#### Scenario: Flag reflects pending mark
- **WHEN** the user marks an installed package for removal and has not yet applied changes
- **THEN** its row shows state/action `i-`

#### Scenario: Flags reset after apply
- **WHEN** a plan containing that removal is successfully applied
- **THEN** the package no longer appears in the list (it is uninstalled)

### Requirement: Broken package detection
An installed package whose dependency tree is invalid (missing or conflicting dependencies) SHALL be flagged with state character `b` and SHALL be locatable via filtering.

#### Scenario: Package with missing dependency
- **WHEN** a globally installed package has a dependency that is absent from its installed tree
- **THEN** its row shows state flag `b` instead of `i`

### Requirement: Outdated detection
The system SHALL determine for each installed package whether the registry's `latest` dist-tag is newer than the installed version, and SHALL display the newer version in the candidate column when so. Upgradability checks MUST NOT block list rendering: if registry data is unavailable, the list SHALL still render from local information with upgradability unknown.

#### Scenario: Outdated package
- **WHEN** an installed package is at 1.2.0 and the registry `latest` dist-tag is 1.4.0
- **THEN** its candidate column shows 1.4.0 and the row matches an "upgradable" filter

#### Scenario: Registry unreachable
- **WHEN** the registry cannot be reached during upgradability checks
- **THEN** the list still renders all installed packages from local data, candidate columns are empty, and a non-fatal notice is shown

### Requirement: Filtering
The user SHALL be able to filter the visible list with aptitude-style expressions supporting at least: `~i` (installed), `~u` (upgradable), `~b` (broken), `~n <pattern>` (name matches), negation `!`, conjunction `&`, and disjunction `|`. An invalid expression SHALL be rejected with an error message and the previous filter retained.

#### Scenario: Filter to upgradable
- **WHEN** the user applies the filter `~u` and two of five installed packages are outdated
- **THEN** only those two rows are visible

#### Scenario: Name pattern filter
- **WHEN** the user applies the filter `~n ^prettier`
- **THEN** only packages whose names start with "prettier" are visible

#### Scenario: Invalid filter expression
- **WHEN** the user submits a syntactically invalid filter expression
- **THEN** an error message is shown and the previously active filter remains in effect

#### Scenario: Filter to broken packages
- **WHEN** the user applies the filter `~b` and one of five installed packages has a broken dependency tree
- **THEN** only that row is visible

### Requirement: Local live matching
The user SHALL be able to initiate a transient name match that, while active, narrows the visible list to installed packages whose names match the typed pattern, with no registry access of any kind (it MUST work fully offline). Cancelling the match restores the full list. This is a navigation aid, not a persistent filter.

#### Scenario: Live matching offline
- **WHEN** the network is unavailable and the user initiates local matching and types "pre"
- **THEN** the visible list narrows to installed packages whose names contain "pre", with the cursor on the first match, and no registry request is made

#### Scenario: Cancelling restores the list
- **WHEN** the user cancels an active local match
- **THEN** the full list (subject to any persistent filter) is restored

### Requirement: Sorting
The list SHALL be sortable by name (default), version, size, and state flag, with the active sort visible to the user.

#### Scenario: Sort by version
- **WHEN** the user selects version sorting
- **THEN** rows are ordered by package version ascending

### Requirement: Status line
The UI SHALL persistently display the selected prefix path, the active filter expression, the number of visible packages, and the count of pending marks.

#### Scenario: Status reflects pending marks
- **WHEN** two packages are marked for install and one for removal
- **THEN** the status line reports three pending marks
