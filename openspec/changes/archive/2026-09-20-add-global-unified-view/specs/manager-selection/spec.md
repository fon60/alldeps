## Purpose

Select which package manager operates in global mode — exactly one active at a time, driven by the managers detected as available — while preserving installed state and pending marks across switches.

## ADDED Requirements

### Requirement: Manager availability and selection
In global mode the system SHALL present the set of package managers available on the machine and let the user select which one to operate with. Exactly one manager SHALL be active at a time; all global-mode listing, marking, and applying target the active manager.

#### Scenario: Selecting among available managers
- **WHEN** both npm and pnpm are detected as available in global mode
- **THEN** the manager switcher lists both and the user can choose which is active

#### Scenario: Single manager
- **WHEN** only one package manager is detected as available
- THEN the switcher shows that single manager as active and no choice is required

### Requirement: Switching preserves state
Switching the active manager SHALL NOT discard installed state or pending marks. Installed state is shared per destination, so switching managers MUST NOT re-list or clear the list; pending marks made under any manager remain present after switching away and back.

#### Scenario: Marks survive a manager switch
- **WHEN** the user marks a package for install under npm, switches to pnpm, then switches back to npm
- **THEN** the earlier npm mark is still pending and the list contents are unchanged

#### Scenario: Installed state shared across managers
- **WHEN** the user switches from npm to pnpm while viewing the same set of destinations
- **THEN** the installed packages shown for a destination are not re-fetched as if empty; the previously known installed state is retained

### Requirement: Active manager display
The currently active package manager SHALL be visible at all times in the status line, alongside the active destination.

#### Scenario: Status line shows manager
- **WHEN** the user is viewing any screen in global mode with pnpm active
- **THEN** the status line shows "pnpm" as the active manager
