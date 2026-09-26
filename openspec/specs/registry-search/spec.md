# registry-search Specification

## Purpose

Provide on-demand discovery of installable packages through the npm registry search API, with results presented in a dedicated search-results view — without maintaining a local copy of the package universe.

## Requirements

### Requirement: Registry search
The user SHALL be able to submit a search query; the system SHALL query the search endpoint of the configured registry and present the results in a dedicated Search tab as rows with state flag `p`, showing package name, latest version, and a short description. The installed package list underneath SHALL remain intact while a search is active. A single query SHALL return a bounded number of results (a page), not an unbounded set.

#### Scenario: Search returns results
- **WHEN** the user searches for "package manager" and the registry returns matches
- **THEN** a Search tab opens showing result rows with state flag `p`, each showing the package name, its latest version, and a short description

#### Scenario: Bounded results
- **WHEN** a query matches more packages than one result page
- **THEN** results are fetched in bounded pages, never as an unbounded set

### Requirement: Search result pagination
Results SHALL be loaded in pages. When the cursor reaches the end of the loaded results and the registry reports more matches, moving further SHALL automatically fetch and merge the next page (infinite scroll), without requiring a new query. The UI SHALL show how many results have been loaded so far out of the total number reported by the registry. A failed page fetch SHALL keep the already-loaded results, report a notice, and allow retrying by reaching the end again.

#### Scenario: Next page loads on reaching the end
- **WHEN** a query matches 543 packages, the first page of 20 is loaded, and the user moves the cursor past the last loaded row
- **THEN** the next page is fetched and merged into the results without a new query

#### Scenario: Loaded over total shown
- **WHEN** 40 of 543 matches for a query have been loaded
- **THEN** the status line shows the query with "40/543"

#### Scenario: Failed page fetch is recoverable
- **WHEN** fetching a later page fails (network error)
- **THEN** the already-loaded results remain, a notice is shown, and reaching the end again retries the fetch

### Requirement: Registry result ordering
Search results SHALL be displayed in exactly the order returned by the registry (relevance ranking). The local list sort (name/version/state/size) MUST NOT be applied to search results, and later pages SHALL be appended after earlier ones without re-ordering. While a search is active, the status line SHALL NOT display the local sort key.

#### Scenario: Results keep relevance order
- **WHEN** the registry returns the results "pad", "pad-component", "@types/pad-left" in that order for a query
- **THEN** the list shows them in exactly that order, regardless of the active local sort key

#### Scenario: Later pages append in order
- **WHEN** the second page of a search is loaded
- **THEN** its results appear after all previously loaded results, in the registry's order

#### Scenario: Local sort not shown while searching
- **WHEN** a search is active and the local sort key is "name"
- **THEN** the status line does not display "sort:name"

### Requirement: Configured registry respected
The system SHALL perform all registry access (search and metadata) against the registry URL configured for the active npm installation, not a hard-coded default.

#### Scenario: Custom registry in use
- **WHEN** the user's npm configuration points at a private or mirror registry
- **THEN** search queries and package metadata requests are sent to that registry

### Requirement: Search results merge with installed set
A search result for a package that is already installed in the selected prefix SHALL NOT produce a duplicate row; while search results are displayed, such a package renders as its authoritative installed row (with its real state flag, versions and pending mark) instead of a `p` row. A result for a not-installed package SHALL be markable for install, defaulting to its latest version.

#### Scenario: Searching for an installed package
- **WHEN** the user searches for a package that is already globally installed in the selected prefix
- **THEN** no duplicate `p` row is added and the package appears once, as its installed row, among the results

#### Scenario: Mark search result for install
- **WHEN** the user marks a not-installed search result with the install action
- **THEN** its row shows state/action `p+` and it is included in the next apply plan

### Requirement: Search results view
A Search tab SHALL display only its own query's results: the not-installed result rows plus any installed packages present in the results (rendered as their installed rows). Installed packages not part of the results are not shown in that tab; they remain visible on the list tab. Pressing / on an active Search tab SHALL clear its loaded results and take a new query in place, without opening another tab. Closing the Search tab (esc or q) discards its unmarked result rows; marked result rows survive as pending marks. While a Search tab is active, initiating a local live match SHALL be refused with a notice (the match applies to the list tab).

#### Scenario: Only results visible while searching
- **WHEN** the user searches for "pad" and the registry returns ten not-installed matches while five packages are installed
- **THEN** the Search tab shows exactly those ten `p` rows, and the list tab still shows the installed packages when returned to

#### Scenario: Clearing restores the installed list
- **WHEN** the user closes a Search tab with esc or q
- **THEN** its unmarked result rows are removed and the previous tab (e.g. the list) is active showing exactly the installed packages plus any marked result rows

#### Scenario: Re-querying in place
- **WHEN** a Search tab for "foo" is active and the user presses / and submits "bar"
- **THEN** the same tab clears its "foo" results and loads the "bar" results; no new tab is opened

#### Scenario: Local match refused during search
- **WHEN** the user initiates a local live match while a Search tab is active
- **THEN** a notice explains that the match applies to the list tab and no match prompt opens

### Requirement: Search results are ephemeral until marked
A new query in a Search tab SHALL replace previously loaded result rows that carry no pending mark; result rows with a pending mark MUST be retained as pending marks regardless of subsequent queries in the same tab. Installed rows are never affected by search replacement.

#### Scenario: New search replaces unmarked results
- **WHEN** a Search tab shows five unmarked "foo" result rows and the user re-queries it with "bar"
- **THEN** the five "foo" rows are removed and replaced by the "bar" results in the same tab

#### Scenario: Marked results survive a new search
- **WHEN** the user marks one "foo" result for install and then re-queries the same tab with "bar"
- **THEN** the pending mark on foo is retained and the foo row remains visible on the list tab, while the Search tab shows the "bar" results

### Requirement: Search failure handling
A failed search (network error, registry error, no results) SHALL display an appropriate message and MUST NOT clear or modify existing list rows, filters, or pending marks.

#### Scenario: Registry unreachable during search
- **WHEN** the user submits a search while the registry is unreachable
- **THEN** an error message is shown and the previously visible rows and all pending marks remain unchanged

#### Scenario: Search with no matches
- **WHEN** a query matches no packages
- **THEN** a "no results" indication is shown and the list state is otherwise unchanged
