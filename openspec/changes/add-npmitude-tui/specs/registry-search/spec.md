## Purpose

Provide on-demand discovery of installable packages through the npm registry search API, with results merged into the package list as not-installed rows — without maintaining a local copy of the package universe.

## ADDED Requirements

### Requirement: Registry search
The user SHALL be able to submit a search query; the system SHALL query the search endpoint of the configured registry and present the results in the package list as rows with state flag `p`, showing package name, latest version, and a short description. A single query SHALL return a bounded number of results (a page), not an unbounded set.

#### Scenario: Search returns results
- **WHEN** the user searches for "package manager" and the registry returns matches
- **THEN** result rows appear with state flag `p`, each showing the package name, its latest version, and a short description

#### Scenario: Bounded results
- **WHEN** a query matches more packages than one result page
- **THEN** only one page of results is shown and the user refines the query to narrow further

### Requirement: Configured registry respected
The system SHALL perform all registry access (search and metadata) against the registry URL configured for the active npm installation, not a hard-coded default.

#### Scenario: Custom registry in use
- **WHEN** the user's npm configuration points at a private or mirror registry
- **THEN** search queries and package metadata requests are sent to that registry

### Requirement: Search results merge with installed set
A search result for a package that is already installed in the selected prefix SHALL NOT produce a duplicate row; the installed row remains authoritative. A result for a not-installed package SHALL be markable for install, defaulting to its latest version.

#### Scenario: Searching for an installed package
- **WHEN** the user searches for a package that is already globally installed in the selected prefix
- **THEN** no duplicate `p` row is added and the existing `i` row remains

#### Scenario: Mark search result for install
- **WHEN** the user marks a not-installed search result with the install action
- **THEN** its row shows state/action `p+` and it is included in the next apply plan

### Requirement: Search results are ephemeral until marked
A new search SHALL replace previously displayed search rows that carry no pending mark; search rows with a pending mark MUST be retained regardless of subsequent searches. Installed rows are never affected by search replacement.

#### Scenario: New search replaces unmarked results
- **WHEN** the user searches for "foo" (five result rows appear) and then searches for "bar"
- **THEN** the five "foo" rows are removed and replaced by the "bar" results

#### Scenario: Marked results survive a new search
- **WHEN** the user marks one "foo" result for install and then searches for "bar"
- **THEN** the marked "foo" row remains in the list with its pending mark, alongside the new "bar" results

### Requirement: Search failure handling
A failed search (network error, registry error, no results) SHALL display an appropriate message and MUST NOT clear or modify existing list rows, filters, or pending marks.

#### Scenario: Registry unreachable during search
- **WHEN** the user submits a search while the registry is unreachable
- **THEN** an error message is shown and the previously visible rows and all pending marks remain unchanged

#### Scenario: Search with no matches
- **WHEN** a query matches no packages
- **THEN** a "no results" indication is shown and the list state is otherwise unchanged
