## Purpose

Manage project-scoped package collections by pointing npmitude at a directory, auto-detecting which package managers apply, and operating on one manager's collection at a time with strict per-manager isolation.

## ADDED Requirements

### Requirement: Entry mode selection
The system SHALL support two entry modes selected by the command-line argument. With no path argument it SHALL launch global mode. Given a path argument (`.`, `./<dir>`, or an absolute directory) it SHALL launch project mode rooted at that directory. A path argument that does not name an existing directory SHALL be rejected with a clear notice and the application SHALL NOT enter project mode.

#### Scenario: Launch global mode
- **WHEN** the user runs npmitude with no path argument
- **THEN** global mode starts exactly as before, targeting the active manager's destinations

#### Scenario: Launch project mode
- **WHEN** the user runs `npmitude .` inside a project directory
- **THEN** project mode starts rooted at that directory

#### Scenario: Invalid path rejected
- **WHEN** the user runs npmitude with a path argument that is not an existing directory
- **THEN** a clear notice is shown and project mode does not start

### Requirement: Project manager detection
On opening a project, the system SHALL detect which package managers apply to it from marker files present in the project: `package.json` for the Node family (with the lockfile type distinguishing npm, yarn, or pnpm), `composer.json` for Composer, and `go.mod` for Go. The set of applicable managers SHALL be presented to the user. A project with a `package.json` but no recognizable lockfile SHALL default to npm.

#### Scenario: Node project detected
- **WHEN** a project contains `package.json` and `pnpm-lock.yaml`
- **THEN** pnpm is reported as an applicable manager for that project

#### Scenario: Composer project detected
- **WHEN** a project contains `composer.json`
- **THEN** composer is reported as an applicable manager for that project

#### Scenario: No lockfile defaults to npm
- **WHEN** a project contains only `package.json` with no recognizable lockfile
- **THEN** npm is reported as the applicable Node-family manager

### Requirement: Adapter switcher in project mode
In project mode the user SHALL be able to select which applicable manager's collection to operate on, with exactly one active at a time. Switching adapters SHALL preserve each adapter's installed state and pending marks.

#### Scenario: Switch between applicable managers
- **WHEN** a project is applicable to both npm and composer and the user switches from npm to composer
- **THEN** the list shows composer's collection for the project and npm's previously made marks remain intact when switching back

### Requirement: Per-adapter isolation
Lists and plans in project mode SHALL be strictly separate per adapter. A package name in one adapter MUST NOT be conflated with the same name in another adapter, and no mark or plan operation of one adapter MAY affect another adapter's collection.

#### Scenario: Same name across adapters is not conflated
- **WHEN** both the npm and composer collections of a project contain a package named `helper`
- **THEN** marking `helper` for removal in the npm adapter does not mark or remove `helper` in the composer adapter

#### Scenario: Plan scoped to one adapter
- **WHEN** the user builds and applies a plan while the composer adapter is active
- **THEN** only composer operations are executed and the npm collection is untouched

### Requirement: Project environment scope
In project mode the managed environment SHALL be the single project (its per-manager module location), not a global prefix. All listing, marking, and applying operations SHALL target that project's collection for the active adapter.

#### Scenario: Operations target the project
- **WHEN** the user installs a package in project mode with the npm adapter active
- **THEN** the package is installed into the project's own module location, not into any global prefix
