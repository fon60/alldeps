## Purpose

Implement aptitude's mark-then-apply workflow for global npm packages: pending marks, a confirmed plan preview, and execution through the machine's npm with streamed progress.

## ADDED Requirements

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

### Requirement: Execution via raw passthrough
The system SHALL execute an approved plan by invoking the selected environment's own package manager for global install, uninstall, or update operations. The plan compiles to batched invocations grouped by operation kind. During execution the interface is suspended and each invocation's output streams directly to the terminal unfiltered; the system MUST NOT parse that output for correctness. If an invocation fails, remaining invocations are not started. Interrupting a running invocation aborts it and the remainder of the plan. On completion (success, failure, or interruption) the system SHALL prompt the user to either return to the interface (triggering the post-apply state re-read) or quit the application. Applying another plan MUST be blocked until the current run finishes.

#### Scenario: Successful install
- **WHEN** the user confirms a plan installing one package
- **THEN** the interface suspends, the package manager's raw output streams to the terminal, and on completion the prompt offers return or quit; returning refreshes the list from disk showing the package installed

#### Scenario: Failure stops the remaining plan
- **WHEN** the first invocation of a multi-invocation plan fails (e.g. network error during download)
- **THEN** the remaining invocations are not started, the diagnostics remain visible in the terminal, and returning refreshes the list from actual on-disk state

#### Scenario: Interruption
- **WHEN** the user interrupts a running invocation
- **THEN** the invocation is terminated, the remainder of the plan is aborted, and the completion prompt is still offered so the user can read what happened before returning

### Requirement: Post-apply truth from disk
After any apply run, the system SHALL re-read the actual installed state of the prefix rather than assuming the plan succeeded in full, and update the list accordingly.

#### Scenario: Partial success
- **WHEN** a two-operation plan completes with one success and one failure
- **THEN** the list shows the successful operation's effect and retains the failed package in its pre-operation state

### Requirement: Concurrency guard
While a plan is executing, further apply requests SHALL be rejected with a notice; pending marks MAY still be edited.

#### Scenario: Double apply attempt
- **WHEN** the user requests apply while another plan is still running
- **THEN** the request is refused with a notice that an operation is in progress

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
