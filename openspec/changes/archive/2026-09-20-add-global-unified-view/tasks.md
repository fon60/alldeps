## 1. Cross-destination list projection

- [x] 1.1 Add a pure domain/app function that groups installed packages by name across all destinations of the active manager, picking the headline (highest-rank destination that has it) and counting additional destinations, verified by unit tests covering multi-destination, single-destination, and empty cases
- [x] 1.2 Render the unified list with the headline version and presence counter columns, verified by a pty run on a multi-version nvm layout showing one row per name with correct headline and count
- [x] 1.3 Add the per-destination detail table (version or absent per destination) with `+`/`-` marking against a specific destination, verified by a unit test that a removal mark on one destination leaves other destinations' marks/state unchanged

## 2. Manager selection

- [x] 2.1 Add `activeManagerID` to the model and a manager switcher driven by detected/available managers (exactly one active), verified by a unit test asserting only one manager is active and switching changes the active manager
- [x] 2.2 Make manager switching preserve installed state (no re-fetch) and all pending marks, verified by a unit test that marks made under one manager survive switching away and back
- [x] 2.3 Show the active manager in the status line alongside the active destination within the 80x24 budget, verified by a pty run showing both in the status line

## 3. Cross-destination apply engine

- [x] 3.1 Generalize plan derivation to ordered (destination, manager) groups from all pending marks, verified by a unit test that mixed marks across two destinations produce two correctly labeled groups
- [x] 3.2 Acquire locks for every destination in the plan before execution and release after, verified by a unit test asserting all touched destinations are locked before the first group runs
- [x] 3.3 Execute each group through its own destination's manager with streamed output and per-group failure isolation, verified by an extended `test/e2e-session.sh` that applies a two-destination plan in one run
- [x] 3.4 Skip and report invalid (one-manager-per-destination) groups instead of executing them, verified by a unit test that a destination marked under two managers is excluded while other groups proceed

## 4. Install-target selection

- [x] 4.1 Resolve eligible destinations for an available-package install and record the mark directly when exactly one is eligible (no popup), verified by a unit test asserting no prompt appears in the single-destination case
- [x] 4.2 Open a multi-select destination popup when more than one destination is eligible and record one install mark per chosen destination, verified by a pty run selecting two destinations from the popup

## 5. Integration verification

- [x] 5.1 Run `go test ./...`, `./build.sh`, and an extended `test/e2e-session.sh` (search → mark across two destinations → plan → apply), verifying all pass and the flow matches the specs
