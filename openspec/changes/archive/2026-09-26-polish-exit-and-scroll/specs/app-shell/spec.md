## ADDED Requirements

### Requirement: Terminal restoration on exit
The program SHALL run in the terminal's alternate screen buffer for its entire lifetime. On any exit — plain quit, quit after the pending-marks confirmation, or quit after an apply run — the terminal SHALL be restored to exactly the state it was in before launch: prior content visible, no residue of the interface drawn by the program, and the shell prompt continuing from where it was.

#### Scenario: Quit restores the previous screen
- **WHEN** the user launches npmitude, browses the package list, and quits with q
- **THEN** the terminal shows exactly what it displayed before launch — no leftover interface — and the shell prompt continues at its pre-launch position

#### Scenario: Every exit path restores
- **WHEN** the user exits via any supported path (q without pending marks, quit confirmation after marking packages, or q after an apply run completes)
- **THEN** the pre-launch terminal content is restored in every case
