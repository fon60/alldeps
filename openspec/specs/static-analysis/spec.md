# static-analysis Specification

## Purpose

Defines how alldeps enforces code quality locally: a mandatory static-analysis gate on every build, version-pinned tooling with no external service dependency, and a reachability-based vulnerability scan before each release.

## Requirements

### Requirement: Build requires passing static analysis
`./build.sh` SHALL run the full static-analysis gate before compilation and SHALL NOT compile or produce a binary when any check reports findings. The gate SHALL comprise a format check, `go vet`, and aggregated static analysis including staticcheck rules.

#### Scenario: Clean tree builds
- **WHEN** the working tree has no formatting, vet, or static-analysis findings
- **THEN** `./build.sh` completes the gate and produces the `./alldeps` binary

#### Scenario: Findings block the build
- **WHEN** any gate check reports one or more findings
- **THEN** `./build.sh` exits with a non-zero status before compilation starts and no new binary is produced

#### Scenario: Lint can run standalone
- **WHEN** a developer runs the lint command directly (without building)
- **THEN** the same gate checks execute and the exit status reflects findings, so issues can be found without a build attempt

### Requirement: Tooling is local and tracks current releases
The lint and vulnerability-scan commands SHALL run entirely on the local machine, resolving each tool to the latest release compatible with the local Go toolchain at run time. Tool versions SHALL NOT be pinned in any repository file. The commands SHALL NOT depend on any external SaaS or hosted analysis service; fetching public open-source module data and public vulnerability database updates is permitted.

#### Scenario: Tools follow a Go toolchain upgrade
- **WHEN** the developer upgrades the local Go toolchain to a new version
- **THEN** the next lint or vulnerability-scan run uses a compatible tool release without any change to repository files

#### Scenario: No version strings in the repository
- **WHEN** the lint and vulnerability-scan commands are inspected
- **THEN** no tool version is pinned in any repository file; resolution happens at run time

#### Scenario: No paid service or account required
- **WHEN** the lint and vulnerability-scan commands are used
- **THEN** no registration, API key, or paid third-party service is needed; the tools are open source and all analysis executes locally

### Requirement: Pre-release vulnerability scan
The release procedure SHALL require a reachability-based vulnerability scan of the module dependency graph (including the Go standard library) before a release is cut. A release MUST NOT proceed while the scan reports known reachable vulnerabilities, unless they are fixed or explicitly accepted.

#### Scenario: Clean scan allows release
- **WHEN** the pre-release scan reports no known reachable vulnerabilities
- **THEN** the release procedure may continue to snapshot and tagging steps

#### Scenario: Reachable vulnerability blocks release
- **WHEN** the scan reports a known vulnerability reachable from alldeps code or its dependencies
- **THEN** the release is blocked until the vulnerable dependency is updated or the risk is explicitly accepted
