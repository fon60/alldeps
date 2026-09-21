## MODIFIED Requirements

### Requirement: Application header
Every screen SHALL display a three-line header at the top of the window: line 1 carrying the application name, its version, and the host name; line 2 listing the actions available on the current screen; line 3 carrying the tab strip (see Tab navigation). All three lines SHALL be rendered as a full-width band on the primary theme color background (spanning the entire terminal width, not just the printed text). The header MUST NOT consume more than three lines on any screen.

#### Scenario: Header on launch
- **WHEN** the user launches npmitude
- **THEN** line 1 shows "npmitude <version> @ <hostname>", line 2 lists the package-list key bindings, and line 3 shows a tab strip containing only the root List tab, all on a primary-color band spanning the full terminal width

#### Scenario: Band spans the full width
- **WHEN** the header is rendered on an 80-column terminal
- **THEN** each of the three header lines occupies exactly 80 columns of background color

#### Scenario: Header follows the screen
- **WHEN** the user opens the plan preview from the list
- **THEN** line 1 is unchanged, line 2 lists the plan screen's actions (apply / cancel), and line 3 shows the Plan tab as active

## ADDED Requirements

### Requirement: Tab navigation
Persistent context screens — the package list, a search result set, a package's info, its versions, its readme, its conflict resolver, the help reference, and the plan preview — SHALL each occupy a tab in the strip. The package-list tab is the root tab and MUST NOT be closed; pressing q on it quits the program (with the existing pending-marks confirmation). Opening a context SHALL append a new tab and activate it; if a tab with the same kind and subject already exists anywhere in the strip, the system SHALL focus that tab instead of creating a duplicate. Closing a non-root tab with q or esc SHALL remove it from the strip and activate its left neighbor. Holding Ctrl while pressing ArrowLeft/ArrowRight or h/l SHALL move between open tabs without closing any; these keys MUST be inert while an overlay is open, a text prompt has focus, or an apply run is in progress. Each tab SHALL retain its own state (cursor position, scroll offset, loaded data) while inactive, and the plan preview SHALL re-render from current marks whenever it is shown. Transient popups — the environment picker, the manager picker, the install-targets popup, the quit confirmation, and the apply-run view — SHALL NOT appear in the strip and MUST block interaction with tabs while open.

#### Scenario: Opening a context adds a tab
- **WHEN** the user opens the info screen for package foo from the list
- **THEN** the strip shows List and Info foo, with Info foo active

#### Scenario: Same subject focuses instead of duplicating
- **WHEN** an Info foo tab is already open but inactive and the user opens info for foo again
- **THEN** the existing Info foo tab becomes active and no duplicate tab is created

#### Scenario: Closing a tab returns to the left neighbor
- **WHEN** the strip shows List, Info foo, Versions foo with Versions foo active, and the user presses q
- **THEN** the strip shows List and Info foo, with Info foo active

#### Scenario: Moving between tabs keeps them open
- **WHEN** the strip shows List, Info foo, Info bar with List active, and the user presses Ctrl+l twice
- **THEN** Info bar is active and all three tabs remain open

#### Scenario: Root tab quit
- **WHEN** only the List tab is open and the user presses q while marks are pending
- **THEN** the quit confirmation appears exactly as before tabs existed

#### Scenario: Popups do not create tabs
- **WHEN** the user opens the environment picker from the list
- **THEN** the strip is unchanged, no tab is added, and tab-movement keys are inert while the picker is open
