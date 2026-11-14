## MODIFIED Requirements

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
