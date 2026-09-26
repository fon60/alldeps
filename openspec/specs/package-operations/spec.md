# package-operations Specification

## Purpose

Implement aptitude's mark-then-apply workflow for global npm packages: pending marks, a confirmed plan preview, and execution through the machine's npm with streamed progress.

## Requirements

### Requirement: Marking operations
The user SHALL be able to mark the selected package for install, removal, upgrade-to-latest, hold (excluded from bulk upgrades), or revert to no-action. Marks are pending until applied and MUST be reflected in the row's action flag immediately.

#### Scenario: Mark installed package for removal
- **WHEN** the user issues the remove mark on an installed package
- **THEN** its action flag becomes `-` and the pending-mark count increases by one

#### Scenario: Toggle mark off
- **WHEN** the user issues the same mark again on an already marked package
- **THEN** the mark is cleared and the action flag returns to `*`

### Requirement: Bulk operations
The user SHALL be able to mark all currently upgradable packages for upgrade in one action, and to clear all pending marks in one action. Held packages MUST NOT be marked by the bulk upgrade action.

#### Scenario: Mark all upgradable
- **WHEN** three of five installed packages are outdated and none is held, and the user issues mark-all-upgradable
- **THEN** exactly those three rows show the upgrade action flag

#### Scenario: Clear all marks
- **WHEN** the user has pending marks on several packages and issues clear-all
- **THEN** every action flag returns to `*` and the pending count is zero

### Requirement: Plan preview before apply
When the user requests applying pending changes, the system SHALL first display a plan summary. The plan SHALL be organized into groups keyed by (destination, manager); each group lists its packages to install (name and target version, approximate download size), to remove (name and freed disk space), and to upgrade (name, from-version to to-version), together with the destination and manager that group targets. Execution MUST require explicit confirmation; cancelling leaves all marks pending. A group whose destination is marked under two different managers SHALL be shown as invalid and excluded from execution until resolved.

#### Scenario: Confirming a mixed plan
- **WHEN** the user requests apply with one install, one removal, and one upgrade pending
- **THEN** a summary listing all three operations, grouped by their destination and manager, is shown and nothing is executed until the user confirms

#### Scenario: Confirming a multi-destination plan
- **WHEN** the user requests apply with an install pending on one destination and a removal pending on another
- **THEN** the summary shows two groups, each labeled with its destination and manager, and nothing is executed until the user confirms

#### Scenario: Cancelling the plan
- **WHEN** the user declines the plan summary
- **THEN** no package-manager operation runs and all marks remain pending

#### Scenario: Quick apply with g,g
- **WHEN** the user presses g on the list (opening the plan preview) and then presses g again on the plan screen
- **THEN** the plan is confirmed and execution begins without any other input

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

### Requirement: Post-apply truth from disk
After any apply run, the system SHALL re-read the actual installed state of the prefix rather than assuming the plan succeeded in full, and update the list accordingly.

#### Scenario: Partial success
- **WHEN** a two-operation plan completes with one success and one failure
- **THEN** the list shows the successful operation's effect and retains the failed package in its pre-operation state

### Requirement: Concurrency guard
While a plan is executing, the apply screen SHALL accept no input other than ctrl+c (which aborts the running invocation). No second apply run MAY start until the current run finishes.

#### Scenario: Keys ignored during execution
- **WHEN** the user presses any key — including another apply request — while a plan is still running
- **THEN** nothing happens, except that ctrl+c aborts the running invocation

### Requirement: Permission failure reporting
If npm fails because the selected prefix is not writable by the current user, the system SHALL display a clear, actionable message identifying the permission problem and how to proceed (e.g. re-running with elevated privileges), rather than only raw npm error output.

#### Scenario: Non-writable system prefix
- **WHEN** applying an install to a system-owned prefix without sufficient privileges fails
- **THEN** the user sees a message stating the prefix is not writable and what to do about it

### Requirement: Exit confirmation
Quitting the application SHALL require explicit confirmation, protecting unapplied pending marks from accidental loss.

#### Scenario: Confirming quit
- **WHEN** the user issues quit while pending marks exist
- **THEN** a confirmation prompt is shown and the application exits only if confirmed

### Requirement: Install target selection
When the user marks an available (not-installed) package for installation in global mode and more than one destination is eligible, the system SHALL present a popup listing the eligible destinations so the user chooses which one or more to install into. When exactly one destination is eligible, no popup SHALL be shown and that single destination SHALL be used.

#### Scenario: Multiple eligible destinations
- **WHEN** the user marks an available package for install and three destinations are eligible
- **THEN** a popup lists the three destinations and the install mark is recorded only against the destination(s) the user selects

#### Scenario: Single eligible destination
- **WHEN** the user marks an available package for install and exactly one destination is eligible
- **THEN** no popup appears and the install mark is recorded against that single destination

### Requirement: One-manager-per-destination guard
Within a single plan, a destination SHALL be operated on by at most one manager. If pending marks would operate the same destination through two different managers, that destination's group SHALL be flagged invalid and its operations SHALL NOT be executed; operations in other groups (other destinations or a single-manager destination) MAY still proceed. Distinct destinations are always compatible even when they use different managers.

#### Scenario: Same destination under two managers is blocked
- **WHEN** the plan contains an npm mark and a yarn mark that both target the same Node destination
- **THEN** that destination's group is flagged invalid and not executed, while groups for other destinations proceed normally

#### Scenario: Different destinations across managers are allowed
- **WHEN** the plan contains an npm mark targeting one Node destination and a yarn mark targeting a different Node destination
- **THEN** both groups are valid and both are executed in the same apply run
