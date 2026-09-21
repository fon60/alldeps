## Purpose

Manage the package dependencies of a Composer (PHP) project — listing installed packages with their exact versions, searching Packagist and detecting outdated packages, applying operations through composer with the project's lifecycle intact, and surfacing solver conflicts with concrete resolution options.

## ADDED Requirements

### Requirement: Installed package listing
In project mode for a Composer project, the system SHALL list the project's direct dependencies (those declared in `composer.json`) with their exact installed versions as recorded in the project's installed-package metadata (`vendor/composer/installed.json`). When the project has no installed vendor directory, the list SHALL be empty and operations SHALL remain available.

#### Scenario: Direct dependencies listed with exact versions
- **WHEN** a Composer project declares three direct dependencies and its installed metadata records their resolved versions
- **THEN** the list shows exactly three rows, each with the package name and its exact installed version (not the declared constraint)

#### Scenario: Fresh project without vendor directory
- **WHEN** a Composer project has `composer.json` but no `vendor/` directory
- **THEN** the list is empty and marking packages for installation remains possible

### Requirement: Packagist search and outdated detection
The system SHALL search available packages through Packagist and determine candidate (newer) versions from Packagist package metadata, preferring the latest stable release. Search and outdated failures MUST NOT block list rendering or first paint; they SHALL surface as non-fatal notices with the list intact.

#### Scenario: Searching Packagist
- **WHEN** the user searches for a query in a Composer project
- **THEN** results are shown from Packagist with package names, latest versions, and descriptions

#### Scenario: Outdated package shows candidate
- **WHEN** an installed package is at 2.3.0 and Packagist reports 2.5.1 as the latest stable release
- **THEN** its candidate column shows 2.5.1 and the row matches an "upgradable" filter

#### Scenario: Registry failure is non-fatal
- **WHEN** Packagist cannot be reached during search or outdated checks
- **THEN** the list still renders from installed metadata, a non-fatal notice is shown, and no list state is mutated

### Requirement: Composer operations with lifecycle scripts
The system SHALL apply marked operations on a Composer project using composer itself in the project directory: installing/upgrading via `composer require <name>:<constraint>` (with `--no-interaction`) and removing via `composer remove <name>`. The project's lifecycle scripts SHALL run as they would for a normal composer invocation (they are not disabled); only interactivity is suppressed.

#### Scenario: Install applied via composer require
- **WHEN** the user marks an available package for installation and applies the plan
- **THEN** the apply screen runs `composer require <name>:<constraint> --no-interaction` in the project directory, lifecycle script output appears as raw passthrough, and after returning the list shows the package installed

#### Scenario: Removal applied via composer remove
- **WHEN** the user marks an installed package for removal and applies the plan
- **THEN** `composer remove <name> --no-interaction` runs and, after returning, the package no longer appears in the list

### Requirement: Conflict detection through solver dry-run
Before a plan is confirmed, the system SHALL probe pending operations by running them through Composer's dependency solver in dry-run mode (no changes to the project). When the solver reports that requirements cannot be satisfied, the affected packages SHALL be marked as conflicted on the list and the plan gate SHALL be triggered exactly as for any resolver-reported conflict.

#### Scenario: Conflicting install is detected before apply
- **WHEN** the user marks a package for installation whose requirement conflicts with the project's platform or another dependency, and opens the plan
- **THEN** the dry-run probe has reported the clash, the package's row carries the conflict indicator, and the plan gate offers to resolve it

#### Scenario: Non-conflicting operations pass the probe
- **WHEN** all pending operations satisfy the solver in dry-run mode
- **THEN** no conflicts are reported and the plan proceeds without the gate

### Requirement: Solver-derived resolution options
Each detected conflict SHALL present concrete resolution options derived from the solver output and package metadata — such as pinning a compatible version of the target package, removing a conflicting root-managed dependency, or skipping the operation — each with a stated consequence. When the solver output cannot be recognized, the system SHALL still present a fallback conflict carrying the raw solver text together with generic options (choose another version, skip). Selecting an option SHALL update the pending marks and re-run the probe in the background without blocking the interface.

#### Scenario: Platform mismatch offers a compatible version
- **WHEN** the solver fails because the target package requires a newer PHP than the project's platform and an older release line of the package supports the current PHP
- **THEN** the resolver offers installing that older release line (with the consequence stating the platform requirement) alongside skipping

#### Scenario: Unrecognized solver output degrades gracefully
- **WHEN** the dry-run probe fails with solver output the system cannot classify
- **THEN** a fallback conflict shows the raw solver text and generic options, and applying still executes the non-conflicting subset while leaving the conflicted operation marked
