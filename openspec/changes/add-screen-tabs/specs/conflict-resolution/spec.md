## MODIFIED Requirements

### Requirement: Per-package conflict resolution screen
Opening a conflicted package SHALL show its conflicts. Each conflict SHALL present one or more resolution options — such as keep/use a specific version, downgrade, remove, or skip (do not install) — each with a stated consequence describing what else changes. Where relevant an option SHALL include an approximate size delta. Selecting an option SHALL update the marks/plan and re-resolve the affected conflicts. The resolution screen SHALL open as its own tab; closing it with q or esc returns to the previous tab, and if that tab is a plan preview whose conflicts remain unresolved, the plan gate SHALL be shown again.

#### Scenario: Resolving by choosing an option
- **WHEN** a package's conflict offers "downgrade to 1.9.4" (consequence: another package drops to a major downgrade) and the user selects it
- **THEN** the marks/plan are updated to reflect the downgrade and the conflict is re-resolved

#### Scenario: Consequences are shown before choosing
- **WHEN** the user opens the resolution screen for a conflicted package
- **THEN** each option's consequence (and size delta where applicable) is visible before the user commits to it

#### Scenario: Closing returns to the opening context
- **WHEN** the user opens the resolver from the plan gate and closes it without choosing an option
- **THEN** the plan tab is active again and its gate reappears because conflicts remain unresolved
