# conflict-resolution Specification

## Purpose

Give the user package-level visibility into and control over conflicts for any package manager — marking conflicted packages, resolving them with explicit options and consequences, gating the plan on unresolved conflicts, and deduplicating redundant Node copies.

## Requirements

### Requirement: Conflict detection and list marking
The system SHALL mark, on the package list, every package involved in an unresolved conflict using a clear, non-intrusive indicator. Marking MUST NOT block navigation or other operations; the user chooses when to open conflict resolution. A package is conflicted when the active adapter's resolver reports an unresolved conflict involving it.

#### Scenario: Conflicted package marked on the list
- **WHEN** the active adapter reports an unresolved conflict involving a package and the user views the list
- **THEN** that package's row carries a distinct conflict indicator while all other rows remain unmarked

#### Scenario: Marking does not block navigation
- **WHEN** a conflicted package is present in the list
- **THEN** the user can still navigate, filter, sort, and mark other packages normally without being forced to resolve it first

### Requirement: Per-package conflict resolution screen
Opening a conflicted package SHALL show its conflicts. Each conflict SHALL present one or more resolution options — such as keep/use a specific version, downgrade, remove, or skip (do not install) — each with a stated consequence describing what else changes. Where relevant an option SHALL include an approximate size delta. Selecting an option SHALL update the marks/plan and re-resolve the affected conflicts.

#### Scenario: Resolving by choosing an option
- **WHEN** a package's conflict offers "downgrade to 1.9.4" (consequence: another package drops to a major downgrade) and the user selects it
- **THEN** the marks/plan are updated to reflect the downgrade and the conflict is re-resolved

#### Scenario: Consequences are shown before choosing
- **WHEN** the user opens the resolution screen for a conflicted package
- **THEN** each option's consequence (and size delta where applicable) is visible before the user commits to it

### Requirement: Plan gate on unresolved conflicts
When the user opens a plan that contains one or more unresolved conflicts, the system SHALL present a popup asking whether to resolve them. Choosing **[Yes]** SHALL open the conflict resolution screen. Choosing **[No]** SHALL show the plan with the conflicting rows marked in the same way as on the main list.

#### Scenario: Gate offers to resolve
- **WHEN** the user opens a plan that has unresolved conflicts and the gate popup appears
- **THEN** selecting [Yes] takes the user to the conflict resolution screen for the affected packages

#### Scenario: Declining shows the marked plan
- **WHEN** the user opens a plan with unresolved conflicts and selects [No] on the gate popup
- **THEN** the plan is displayed with the conflicting rows carrying the same conflict indicator used on the main list

### Requirement: Apply proceeds with the non-conflicting subset
When a plan containing unresolved conflicts is applied, the system SHALL execute only the operations not involved in an unresolved conflict and SHALL leave the conflicting operations marked (not executed). A note identifying the skipped conflicting operations SHALL be recorded.

#### Scenario: Non-conflicting operations apply, conflicting ones stay marked
- **WHEN** a plan has two independent installs and one install that is part of an unresolved conflict, and the user applies without resolving
- **THEN** the two independent installs are executed, the conflicting install is not executed and remains marked, and a note records that it was skipped

### Requirement: Node dedupe and align flavor
For the Node family, conflict resolution SHALL additionally surface redundant or duplicate copies of a package across destinations and offer options to align versions, consolidate, or remove copies in order to reduce disk usage. This view SHALL reuse the per-destination detail table so the user sees each destination's copy and its size before choosing.

#### Scenario: Duplicate copies surfaced for dedupe
- **WHEN** the same package is installed at different versions on two Node destinations
- **THEN** the resolution view lists both copies with their versions and sizes and offers align/consolidate/remove options

#### Scenario: Aligning reduces duplication
- **WHEN** the user chooses to align a duplicated package to a single version across destinations
- **THEN** the plan marks the differing destination(s) for change so that applying removes the redundant copy and frees the reported space
