# app-shell Specification

## Purpose

Application-wide chrome shared by every screen: an identifying header and a key-binding reference.

## Requirements

### Requirement: Application header
Every screen SHALL display a two-line header at the top of the window: line 1 carrying the application name, its version, and the host name; line 2 listing the actions available on the current screen. Both lines SHALL be rendered as a full-width band on the primary theme color background (spanning the entire terminal width, not just the printed text). The header MUST NOT consume more than two lines on any screen.

#### Scenario: Header on launch
- **WHEN** the user launches npmitude
- **THEN** line 1 shows "npmitude <version> @ <hostname>" and line 2 lists the package-list key bindings, both on a primary-color band spanning the full terminal width

#### Scenario: Band spans the full width
- **WHEN** the header is rendered on an 80-column terminal
- **THEN** each of the two header lines occupies exactly 80 columns of background color

#### Scenario: Header follows the screen
- **WHEN** the user opens the plan preview from the list
- **THEN** line 1 is unchanged and line 2 lists the plan screen's actions (apply / cancel)

### Requirement: Help screen
Pressing `?` on any screen SHALL open a help screen listing all key bindings grouped by context. Closing it with esc, q, or enter SHALL return to the screen it was opened from. On screens where the content exceeds the visible area, the help screen SHALL be scrollable.

#### Scenario: Open and close help
- **WHEN** the user presses `?` on the package list and then presses esc
- **THEN** the help screen shows all key bindings grouped by context and the user returns to the package list

#### Scenario: Help from a sub-screen
- **WHEN** the user opens help from the environment picker and closes it
- **THEN** the user returns to the environment picker, not to the package list

### Requirement: Terminal restoration on exit
The program SHALL run in the terminal's alternate screen buffer for its entire lifetime. On any exit — plain quit, quit after the pending-marks confirmation, or quit after an apply run — the terminal SHALL be restored to exactly the state it was in before launch: prior content visible, no residue of the interface drawn by the program, and the shell prompt continuing from where it was.

#### Scenario: Quit restores the previous screen
- **WHEN** the user launches npmitude, browses the package list, and quits with q
- **THEN** the terminal shows exactly what it displayed before launch — no leftover interface — and the shell prompt continues at its pre-launch position

#### Scenario: Every exit path restores
- **WHEN** the user exits via any supported path (q without pending marks, quit confirmation after marking packages, or q after an apply run completes)
- **THEN** the pre-launch terminal content is restored in every case
