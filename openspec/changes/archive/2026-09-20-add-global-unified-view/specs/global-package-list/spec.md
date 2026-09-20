## MODIFIED Requirements

### Requirement: Installed package listing
In global mode the system SHALL display one row per package *name*, aggregated across all destinations of the active manager, rather than one row per (destination, package). For each name the row SHALL show a two-character state/action flag, the package name, the installed version taken from the headline destination, disk size, and candidate version when different. The **headline destination** is the highest-ranked destination (by Node/runtime version, not by package version) that has the package installed; a **presence counter** SHALL show how many additional destinations also have the package installed.

#### Scenario: Listing globals on launch
- **WHEN** the user launches npmitude in global mode and a single destination of the active manager has three globally installed packages
- **THEN** the list shows exactly three rows, each with state flag `i`, the package name, its installed version (the headline copy), and its disk size

#### Scenario: Aggregated row across destinations
- **WHEN** package `foo` is installed on three destinations of the active manager
- **THEN** the list shows exactly one row for `foo`, its headline version is the copy from the highest-ranked destination that has it, and its presence counter shows two additional destinations

#### Scenario: Package in a single destination
- **WHEN** package `bar` is installed on only one destination of the active manager
- **THEN** the list shows one row for `bar` with a presence counter of zero additional destinations

#### Scenario: Empty prefix
- **WHEN** no destination of the active manager has any globally installed packages
- **THEN** the list is empty and the status line reports zero packages

## ADDED Requirements

### Requirement: Per-destination detail view
A package's details SHALL include a traversable table listing, for each destination of the active manager, the version of that package installed there or an indicator that it is absent. From this table the user SHALL be able to mark the package for installation into, or removal from, a specific destination without affecting its presence in other destinations.

#### Scenario: Viewing per-destination versions
- **WHEN** the user opens the details of a package installed on two destinations at different versions
- **THEN** the table shows one entry per destination with that destination's installed version, and an absent indicator for any destination lacking it

#### Scenario: Mark removal on one destination only
- **WHEN** the user marks the package for removal against a single destination in the detail table
- **THEN** only that destination gets the removal mark and the package's presence on the other destinations is unchanged
