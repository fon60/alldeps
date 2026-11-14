# go-module-management Specification

## Purpose
Manage the module dependencies of a Go project — listing direct requirements with their resolved versions, detecting outdated modules through the Go toolchain itself, and applying install/upgrade/remove operations via `go get`.

## Requirements

### Requirement: Module listing from the build list
In project mode for a Go project, the system SHALL list every module in the project's build list with its resolved version as selected by the module graph, not merely the constraints declared in `go.mod`. Direct requirements and indirect (transitive) dependencies SHALL both be listed; indirect modules SHALL carry the automatic marker in their flag. The main module itself SHALL NOT be listed.

#### Scenario: Direct requirements listed with resolved versions
- **WHEN** a Go project's build list contains three direct modules and five indirect ones
- **THEN** the list shows those three direct modules, each with the module path and its resolved (selected) version

#### Scenario: Indirect dependencies listed and marked automatic
- **WHEN** a Go project's build list contains five indirect (transitive) modules pulled in by its direct requirements
- **THEN** the list also shows those five indirect modules, each carrying the automatic marker `A`

#### Scenario: Main module and indirect deps excluded
- **WHEN** the user views the list of a Go project
- **THEN** the main module itself is not shown as a row (indirect dependencies are no longer excluded — they are listed and marked `A`, see the adjacent scenario)

### Requirement: Outdated detection through the Go toolchain
The system SHALL determine candidate (newer) versions for listed modules using the Go toolchain's own update information, staying within the module's current major-version line. Outdated checks MUST NOT block list rendering; when they fail (e.g. network unavailable), the list SHALL still render with unknown candidates and a non-fatal notice.

#### Scenario: Outdated module shows candidate
- **WHEN** a listed module is at v1.2.0 and the toolchain reports v1.4.2 as available within the same major line
- **THEN** its candidate column shows v1.4.2 and the row matches an "upgradable" filter

#### Scenario: Update check fails non-fatally
- **WHEN** the update check cannot reach the module proxy
- **THEN** the list still renders all modules from the build list, candidate columns are empty, and a non-fatal notice is shown

### Requirement: Go toolchain requirement
Listing and operations for a Go project SHALL use the `go` toolchain available on the machine's PATH. When no usable `go` toolchain is available, loading the project SHALL fail with a clear notice identifying the missing toolchain rather than an opaque error.

#### Scenario: Missing toolchain reported clearly
- **WHEN** the user opens a Go project and no `go` binary is on PATH
- **THEN** a clear notice states that the Go toolchain is required and unavailable, and no module list is shown

### Requirement: Module operations via go get
The system SHALL apply marked operations on a Go project using `go get`: installing a module at a given version (or latest when unversioned) and upgrading to a target version with `go get <module>@<version>`, and removing a module with `go get <module>@none`. Operations SHALL run in the project directory and SHALL be surgical: a removal MUST NOT rewrite unrelated requirements (no automatic `go mod tidy`).

#### Scenario: Upgrade applied via go get
- **WHEN** the user marks a listed module for upgrade to v1.4.2 and applies the plan
- **THEN** the apply screen runs `go get <module>@v1.4.2` in the project directory and, after returning, the list shows the new resolved version

#### Scenario: Removal is surgical
- **WHEN** the user marks one module for removal and applies the plan among other pending operations
- **THEN** only that module's requirement is dropped; requirements the user did not mark are left untouched in `go.mod`

### Requirement: Search unavailable for Go modules
The Go ecosystem SHALL advertise that it has no package-search capability. The system MUST NOT present registry search as available in a Go project; invoking the search entry point SHALL produce a clear "search not available" notice instead of an error state or a failing query.

#### Scenario: Invoking search shows a notice
- **WHEN** the user invokes the search entry point while a Go project is open
- **THEN** a notice states that search is not available for Go modules and no registry query is attempted
