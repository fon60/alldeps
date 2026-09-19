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
When the user requests applying pending changes, the system SHALL first display a plan summary listing: packages to install (name and target version, approximate download size), packages to remove (name and freed disk space), and packages to upgrade (name, from-version to to-version). Execution MUST require explicit confirmation; cancelling leaves all marks pending.

#### Scenario: Confirming a mixed plan
- **WHEN** the user requests apply with one install, one removal, and one upgrade pending
- **THEN** a summary listing all three operations with their sizes is shown and nothing is executed until the user confirms

#### Scenario: Cancelling the plan
- **WHEN** the user declines the plan summary
- **THEN** no npm operation runs and all marks remain pending

#### Scenario: Quick apply with g,g
- **WHEN** the user presses g on the list (opening the plan preview) and then presses g again on the plan screen
- **THEN** the plan is confirmed and execution begins without any other input

### Requirement: Apply progress screen
The system SHALL execute an approved plan by invoking the selected environment's own package manager for global install, uninstall, or update operations. The plan compiles to batched invocations grouped by operation kind. During execution the interface stays up and a dedicated full-screen view replaces the list: each invocation is shown as its command line followed by its raw combined output (stdout and stderr unfiltered, in order), auto-scrolled so the newest output is visible; the system MUST NOT parse that output for correctness. A bottom line SHALL always state what is happening or what to do next: while a batch runs, which step of how many is executing with its command; after the last batch, that the list is being re-read from disk; on completion (success or failure), the prompt to return to the interface or quit. If an invocation fails, remaining invocations are not started and its error is recorded in the log. Pressing ctrl+c while a batch runs SHALL abort that invocation and the remainder of the plan. Applying another plan MUST be blocked until the current run finishes; all other keys are ignored while the apply screen is up.

#### Scenario: Successful install
- **WHEN** the user confirms a plan installing one package
- **THEN** the apply screen shows the command line and the package manager's raw output as it runs, then a completion view with the prompt to return or quit; returning shows the refreshed list with the package installed

#### Scenario: Failure stops the remaining plan
- **WHEN** the first invocation of a multi-invocation plan fails (e.g. network error during download)
- **THEN** the error is recorded in the apply screen's log, the remaining invocations are not started, and returning refreshes the list from actual on-disk state

#### Scenario: Interruption
- **WHEN** the user presses ctrl+c while a batch is running
- **THEN** the invocation is terminated, its error is recorded in the log, the remainder of the plan is aborted, and the completion prompt is still offered so the user can read what happened before returning

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
