## MODIFIED Requirements

### Requirement: Viewport scrolling
When the visible list exceeds the number of rows that fit on screen, the list viewport SHALL follow the cursor: moving the cursor past the bottom edge scrolls the list down, and moving it back above the top edge scrolls it up. The cursor row MUST always be visible; when the cursor reaches the bottom edge of the viewport, the viewport SHALL scroll so that the cursor row is the last data row printed (the column-header line does not count toward the number of data rows that fit).

#### Scenario: Scrolling down follows the cursor
- **WHEN** 30 rows are visible on a screen whose list area fits 16 data rows and the user moves the cursor to row 20
- **THEN** the viewport has scrolled so that row 20 (the cursor row) is visible at the bottom edge

#### Scenario: Cursor lands on the last visible row
- **WHEN** the viewport is full and the user moves the cursor one row past the currently visible range
- **THEN** the viewport scrolls by exactly one row so that the cursor row is the last data row printed, and no row beyond it is rendered

#### Scenario: Scrolling back up follows the cursor
- **WHEN** the viewport is scrolled down and the user moves the cursor above the currently visible range
- **THEN** the viewport scrolls up so the cursor row is visible again
