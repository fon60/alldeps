## ADDED Requirements

### Requirement: Node toolchain pinning in project mode
In project mode, when the project root contains an `.nvmrc` file whose first line pins a Node version, the system SHALL resolve that pin against the installed Node prefixes (nvm/fnm/volta). A full version (e.g. `20.19.1`, with or without a leading `v`) SHALL match only the prefix of exactly that version; a partial version (major or major.minor, e.g. `22` or `22.15`) SHALL match the highest installed version having that prefix. When a matching prefix exists, all project-scoped operations (listing, registry resolution, execution) SHALL use that prefix's node and npm instead of the active shell prefix.

#### Scenario: Partial pin resolves to the best installed version
- **WHEN** a project's `.nvmrc` contains `22` and the machine has nvm prefixes v20.19.1 and v22.15.0 installed
- **THEN** project operations run with the v22.15.0 prefix's node and npm

#### Scenario: Full pin resolves exactly
- **WHEN** a project's `.nvmrc` contains `v20.19.1` and both v20.19.1 and v20.3.0 are installed
- **THEN** project operations run with the v20.19.1 prefix's node and npm

#### Scenario: Pin does not change global mode
- **WHEN** the user launches npmitude without a path argument in a shell whose cwd contains an `.nvmrc`
- **THEN** global mode behaves exactly as before, ignoring any `.nvmrc`

### Requirement: Fallback when the pinned version is not installed
When an `.nvmrc` pin matches no installed Node prefix, the system SHALL fall back to the active shell prefix's toolchain (the current behavior) and SHALL show a non-blocking notice stating that the pinned version is not installed. Project operations SHALL proceed; the missing pin SHALL NOT block listing, marking, or execution.

#### Scenario: Missing pinned version falls back with a notice
- **WHEN** a project's `.nvmrc` contains `24` and no v24.x prefix is installed
- **THEN** project operations run with the active shell prefix's toolchain and a notice reports that the pinned Node 24 is not installed

### Requirement: Absent or unparseable pin leaves behavior unchanged
When the project root has no `.nvmrc`, or its first line does not parse as a Node version (aliases, comments, empty file), the system SHALL use the active shell prefix's toolchain with no pinning notice.

#### Scenario: No .nvmrc present
- **WHEN** a project directory contains `package.json` but no `.nvmrc`
- **THEN** project operations run with the active shell prefix's toolchain and no notice is shown

#### Scenario: Unparseable pin ignored
- **WHEN** a project's `.nvmrc` first line is an alias such as `lts/*`
- **THEN** the pin is ignored, the active shell prefix's toolchain is used, and no pinning notice is shown
