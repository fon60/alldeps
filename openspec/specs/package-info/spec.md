# package-info Specification

## Purpose

Provide a detail screen for a single package showing its metadata, dependency list, and published versions, sourced from local installation data plus registry metadata.

## Requirements

### Requirement: Package detail display
For the selected package, the system SHALL display: name, version (installed if installed, otherwise latest), description, homepage or repository URL, license, maintainers, bin entries (when installed), disk usage (when installed), and its direct dependency list with version ranges. Missing metadata fields SHALL be shown as absent without breaking the screen.

#### Scenario: Installed package details
- **WHEN** the user opens the info screen for an installed package
- **THEN** the screen shows its description, license, bin entries, disk usage, and dependencies with version ranges

#### Scenario: Package without description
- **WHEN** the selected package's metadata has no description field
- **THEN** the info screen still renders all other fields and indicates the description is absent

### Requirement: Dependency listing
The dependency list SHALL distinguish dependency types (dependencies vs peerDependencies) by label.

#### Scenario: Peer dependencies shown separately
- **WHEN** a package declares both dependencies and peerDependencies
- **THEN** the info screen lists them under separate labeled sections

### Requirement: Version history
The user SHALL be able to view the published versions of a package and mark a specific version using + (install/upgrade at exactly that version) or - (removal). The version list SHALL be openable with v both from the package list (for the highlighted row, including search results) and from the info screen. The list SHALL resemble the main package list: one row per published version, newest first, showing a three-character flag (state i when that exact version is installed in at least one environment, else p; the automatic marker A when the installed presence is an automatic dependency, else a space; plus the pending mark's action character — a space when none — on the row whose version the mark targets), the version, a size (the measured disk size when that version is installed somewhere, otherwise the registry-reported unpacked size, otherwise an unknown indicator), and a where column naming the highest-ranked environment carrying that exact version with a presence counter for further environments holding it (an absent indicator when installed nowhere). The list SHALL be scrollable with cursor-following viewport behavior identical to the package list, so every published version is reachable regardless of list length. The enter and space keys SHALL NOT perform a marking action on this screen; only + and - mark a version.

#### Scenario: Browsing versions
- **WHEN** the user highlights a package on the main list and presses v (or opens it from the info screen)
- **THEN** a Versions tab opens showing that package's published versions newest first, each with its flag, size where known, and the environments carrying it

#### Scenario: Per-environment status shown
- **WHEN** version 18.19.0 of a package is installed on four Node environments, the highest-ranked of which is v22
- **THEN** that version's row shows state i and its where column reads v22 with a +3 presence counter

#### Scenario: Version not installed anywhere
- **WHEN** a published version is installed in no environment
- **THEN** its row shows state p and an absent indicator in the where column

#### Scenario: Long history is scrollable
- **WHEN** a package has more published versions than fit on screen
- **THEN** the list scrolls with the cursor following, keeping the cursor row visible exactly as the package list does

#### Scenario: Mark specific version for install
- **WHEN** the user presses + on version 1.2.3 from the version list of a not-installed package
- **THEN** the package is marked for install at exactly 1.2.3 and the plan preview shows that pinned version

### Requirement: Info screen failure handling
If registry metadata for the selected package cannot be fetched, the info screen SHALL still render from locally available data (for installed packages) and indicate that remote details are unavailable.

#### Scenario: Registry unreachable while viewing info
- **WHEN** the user opens the info screen for a not-installed package while the registry is unreachable
- **THEN** an unavailability notice is shown instead of fabricated or stale remote data

### Requirement: Detail screens as tabs
The package info screen, the published-versions screen, and the readme screen SHALL each open as its own tab rather than replacing the current view in place: info opens from the list or a search result; versions and readme open from the info tab. Closing any of them with q or esc removes its tab and activates the previous tab (info for versions/readme, the opening context for info). Each retains its own state while inactive, and re-opening the same package's screen focuses the existing tab instead of duplicating it.

#### Scenario: Info then versions shows both tabs
- **WHEN** the user opens info for foo and then opens its version list
- **THEN** the strip shows List, Info foo, Versions foo with Versions foo active, and pressing esc returns to Info foo with its state intact

#### Scenario: Re-opening focuses the existing tab
- **WHEN** an Info foo tab exists (inactive) and the user opens info for foo from the list again
- **THEN** the existing Info foo tab is activated, not a new one

### Requirement: Version history availability per ecosystem
Every supported ecosystem SHALL provide the full published version list of a package in one fetch (no pagination): npm from the registry package document, Composer from the Packagist package document, Go modules from the module's tagged versions. A failure to fetch versions SHALL NOT block the screen: the view SHALL render whatever is available locally and indicate that the version history is unavailable.

#### Scenario: Go module version list
- **WHEN** the user opens the version list for a Go module dependency in project mode
- **THEN** the tagged versions of that module are listed, newest first

#### Scenario: Version fetch failure degrades gracefully
- **WHEN** the version list cannot be fetched (network error or unsupported ecosystem)
- **THEN** the view shows a non-fatal unavailability notice and no fabricated versions

### Requirement: Version-level install/remove marking
On the versions screen, the user SHALL be able to mark the version under the cursor using two orthogonal actions. Pressing + SHALL mark that exact version for installation (a toggle: pressing it again clears the install mark); it SHALL NOT produce a removal of an already-installed package. Pressing - SHALL mark the package for removal (a toggle: pressing it again cancels the removal mark); it SHALL NOT produce an install. When more than one environment is eligible for the action, the system SHALL present a scrollable list of those environments; within that list + includes an environment in the action and - excludes or cancels it (for a removal, - marks the environment for removal and + cancels that removal). The enter and space keys SHALL NOT mark a version.

#### Scenario: Plus marks a specific version for install
- **WHEN** the user presses + on version 1.2.3 of a not-installed package and exactly one environment is eligible
- **THEN** the package is marked for install at exactly 1.2.3 in that environment, with no popup

#### Scenario: Install mark toggles off
- **WHEN** the user presses + on a version already marked for install
- **THEN** the install mark is cleared and the action flag returns to no-action

#### Scenario: Plus never removes an installed package
- **WHEN** the user presses + while the package is already installed
- **THEN** no removal mark is created; only the install/upgrade mark for the selected version is managed

#### Scenario: Mark a version for removal
- **WHEN** the user presses - on the installed version of a package and exactly one environment has it installed
- **THEN** the package is marked for removal in that environment, with no popup

#### Scenario: Minus never installs
- **WHEN** the user presses - on a not-installed version row
- **THEN** no install mark is created

#### Scenario: Enter or space does not mark
- **WHEN** the user presses enter or space on a version row
- **THEN** no mark is created or changed and the cursor/screen state is unchanged

#### Scenario: Multi-environment install popup
- **WHEN** the user presses + on a version of a not-installed package and three environments are eligible
- **THEN** a scrollable list of the three environments is shown, where + selects an environment to install into and - deselects it, and confirming records the install mark only for the selected environments

#### Scenario: Multi-environment removal popup
- **WHEN** the user presses - on a package installed in two environments
- **THEN** a scrollable list of those environments is shown, where - marks an environment for removal and + cancels that removal
