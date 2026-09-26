## MODIFIED Requirements

### Requirement: State and action flags
Each row SHALL display a three-character flag of the form `<state><auto><action>`. The state character is `i` (installed), `p` (available-not-installed), or `b` (broken). The auto character is `A` when the package was installed automatically as a dependency and a space when it was installed directly. The action character reflects only marks that have not yet been applied: a space for no pending mark, `+` install, `-` remove, `u` upgrade, or `h` held. Where a slot is empty it is rendered as a space.

#### Scenario: Flag reflects pending mark
- **WHEN** the user marks an installed, directly-installed package for removal and has not yet applied changes
- **THEN** its row shows the flag `i -` (installed, direct, marked for removal)

#### Scenario: Automatically installed dependency flag
- **WHEN** a package was pulled in as a dependency rather than installed directly and has no pending mark
- **THEN** its row shows the flag `iA ` (installed, automatic, no action)

#### Scenario: Automatic dependency marked for removal
- **WHEN** an automatically installed package is marked for removal and not yet applied
- **THEN** its row shows the flag `iA-` (installed, automatic, marked for removal)

#### Scenario: Flags reset after apply
- **WHEN** a plan containing that removal is successfully applied
- **THEN** the package no longer appears in the list (it is uninstalled)

## ADDED Requirements

### Requirement: Automatic dependency visibility
The package list SHALL include packages that were installed automatically as dependencies of other packages, in addition to directly installed packages, so that every installed package is visible by default. Automatically installed packages SHALL be distinguishable from directly installed ones via the auto character of the flag. The user SHALL be able to toggle between showing all packages (the default) and showing only directly installed packages; when the manual-only view is active, automatically installed packages are hidden.

#### Scenario: Dependencies visible by default
- **WHEN** a directly installed package pulls in three dependencies that are not themselves directly installed
- **THEN** the list shows the direct package plus its three dependencies, each dependency carrying the automatic marker `A`

#### Scenario: Manual-only toggle hides automatic packages
- **WHEN** the user activates the manual-only view on a list containing both direct and automatic packages
- **THEN** only the directly installed packages remain visible

#### Scenario: Toggle default is show all
- **WHEN** the user opens the package list without changing the view
- **THEN** both directly installed and automatically installed packages are shown
