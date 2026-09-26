## MODIFIED Requirements

### Requirement: Registry search
The user SHALL be able to submit a search query; the system SHALL query the search endpoint of the configured registry and present the results in a dedicated Search tab as rows with state flag `p`, showing package name, latest version, and a short description. The installed package list underneath SHALL remain intact while a search is active. A single query SHALL return a bounded number of results (a page), not an unbounded set.

#### Scenario: Search returns results
- **WHEN** the user searches for "package manager" and the registry returns matches
- **THEN** a Search tab opens showing result rows with state flag `p`, each showing the package name, its latest version, and a short description

#### Scenario: Bounded results
- **WHEN** a query matches more packages than one result page
- **THEN** results are fetched in bounded pages, never as an unbounded set

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
