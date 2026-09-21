## MODIFIED Requirements

### Requirement: Version history
The user SHALL be able to view the published versions of a package and select a specific version to mark the package for install at exactly that version. The version list SHALL be openable with v both from the package list (for the highlighted row, including search results) and from the info screen. The list SHALL resemble the main package list: one row per published version, newest first, showing a two-character flag (state `i` when that exact version is installed in at least one environment, else `p`; plus the pending mark's action character on the row whose version the mark targets), the version, a size (the measured disk size when that version is installed somewhere, otherwise the registry-reported unpacked size, otherwise an unknown indicator), and a where column naming the highest-ranked environment carrying that exact version with a presence counter for further environments holding it (an absent indicator when installed nowhere). The list SHALL be scrollable with cursor-following viewport behavior identical to the package list, so every published version is reachable regardless of list length. Selecting a version with enter SHALL mark the package for install or upgrade at exactly that version, as before.

#### Scenario: Browsing versions
- **WHEN** the user highlights a package on the main list and presses v (or opens it from the info screen)
- **THEN** a Versions tab opens showing that package's published versions newest first, each with its flag, size where known, and the environments carrying it

#### Scenario: Per-environment status shown
- **WHEN** version 18.19.0 of a package is installed on four Node environments, the highest-ranked of which is v22
- **THEN** that version's row shows state `i` and its where column reads v22 with a +3 presence counter

#### Scenario: Version not installed anywhere
- **WHEN** a published version is installed in no environment
- **THEN** its row shows state `p` and an absent indicator in the where column

#### Scenario: Long history is scrollable
- **WHEN** a package has more published versions than fit on screen
- **THEN** the list scrolls with the cursor following, keeping the cursor row visible exactly as the package list does

#### Scenario: Mark specific version for install
- **WHEN** the user selects version 1.2.3 from the version list of a not-installed package
- **THEN** the package is marked for install at exactly 1.2.3 and the plan preview shows that pinned version

## ADDED Requirements

### Requirement: Version history availability per ecosystem
Every supported ecosystem SHALL provide the full published version list of a package in one fetch (no pagination): npm from the registry package document, Composer from the Packagist package document, Go modules from the module's tagged versions. A failure to fetch versions SHALL NOT block the screen: the view SHALL render whatever is available locally and indicate that the version history is unavailable.

#### Scenario: Go module version list
- **WHEN** the user opens the version list for a Go module dependency in project mode
- **THEN** the tagged versions of that module are listed, newest first

#### Scenario: Version fetch failure degrades gracefully
- **WHEN** the version list cannot be fetched (network error or unsupported ecosystem)
- **THEN** the view shows a non-fatal unavailability notice and no fabricated versions
