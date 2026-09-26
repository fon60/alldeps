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
The user SHALL be able to view the published versions of a package from registry metadata, and SHALL be able to select a specific version to mark the package for install at exactly that version.

#### Scenario: Browsing versions
- **WHEN** the user opens the version list for a package
- **THEN** the published versions are listed with the currently installed (or latest) version identifiable

#### Scenario: Mark specific version for install
- **WHEN** the user selects version 1.2.3 from the version list of a not-installed package
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
