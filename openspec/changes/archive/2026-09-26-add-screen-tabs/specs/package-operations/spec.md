## MODIFIED Requirements

### Requirement: Apply progress screen
The system SHALL execute an approved plan by invoking each destination's own package manager for its group of install, uninstall, or update operations. The plan compiles to groups keyed by (destination, manager), and each group is executed through that destination's manager as batched invocations grouped by operation kind. Before execution begins the system SHALL acquire locks for every destination the plan touches. During execution the interface stays up and a dedicated full-screen view replaces the list: each invocation is shown as its command line followed by its raw combined output (stdout and stderr unfiltered, in order), auto-scrolled so the newest output is visible; the system MUST NOT parse that output for correctness. A bottom line SHALL always state what is happening or what to do next: while a batch runs, which step of how many is executing with its command; after the last batch, that the list is being re-read from disk; on completion (success or failure), the prompt to return to the interface or quit. If an invocation fails, remaining invocations in that group are not started and its error is recorded in the log. Pressing ctrl+c while a batch runs SHALL abort that invocation and the remainder of the plan. Applying another plan MUST be blocked until the current run finishes; all other keys are ignored while the apply screen is up. The apply view SHALL NOT appear in the tab strip and MUST block all tab navigation for its duration, including after completion until the user returns or quits.

#### Scenario: One run spans multiple destinations
- **WHEN** the user confirms a plan with an install on one Node destination and a removal on another
- **THEN** the apply screen runs both groups in the same execution, each through its own destination's manager, and locks for both destinations are held for the duration

#### Scenario: Successful install
- **WHEN** the user confirms a plan installing one package
- **THEN** the apply screen shows the command line and the package manager's raw output as it runs, then a completion view with the prompt to return or quit; returning shows the refreshed list with the package installed

#### Scenario: Failure stops the remaining plan
- **WHEN** an invocation of a group fails (e.g. network error during download)
- **THEN** the error is recorded in the apply screen's log, the remaining invocations of that group are not started, and returning refreshes the list from actual on-disk state

#### Scenario: Interruption
- **WHEN** the user presses ctrl+c while a batch is running
- **THEN** the invocation is terminated, its error is recorded in the log, the remainder of the plan is aborted, and the completion prompt is still offered so the user can read what happened before returning

#### Scenario: Apply run blocks tab navigation
- **WHEN** an apply run is in progress, or finished but not yet dismissed, and the user presses Ctrl+l
- **THEN** no tab change occurs; only the apply view's own keys act
