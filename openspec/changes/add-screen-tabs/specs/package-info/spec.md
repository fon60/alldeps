## ADDED Requirements

### Requirement: Detail screens as tabs
The package info screen, the published-versions screen, and the readme screen SHALL each open as its own tab rather than replacing the current view in place: info opens from the list or a search result; versions and readme open from the info tab. Closing any of them with q or esc removes its tab and activates the previous tab (info for versions/readme, the opening context for info). Each retains its own state while inactive, and re-opening the same package's screen focuses the existing tab instead of duplicating it.

#### Scenario: Info then versions shows both tabs
- **WHEN** the user opens info for foo and then opens its version list
- **THEN** the strip shows List, Info foo, Versions foo with Versions foo active, and pressing esc returns to Info foo with its state intact

#### Scenario: Re-opening focuses the existing tab
- **WHEN** an Info foo tab exists (inactive) and the user opens info for foo from the list again
- **THEN** the existing Info foo tab is activated, not a new one
