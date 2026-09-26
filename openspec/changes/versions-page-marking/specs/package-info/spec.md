## MODIFIED Requirements

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

## ADDED Requirements

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
