## 1. Conflict state and detection

- [x] 1.1 Store conflict state per (destination, package) in the domain, populated from the adapter's `Resolve`, verified by a unit test that a stub adapter reporting a conflict surfaces it for the affected package
- [x] 1.2 Confirm npm's resolver reports no conflicts today so the feature is inert for npm except dedupe, verified by a unit test asserting an empty conflict set from the npm adapter

## 2. List marking

- [x] 2.1 Mark conflicted rows on the list with a distinct non-intrusive indicator and add a conflict filter, verified by a pty run showing the marker without blocking navigation
- [x] 2.2 Verify marking does not block other operations (navigate/filter/sort/mark others), verified by a unit test asserting normal operations proceed while a conflict is present

## 3. Per-package resolver screen

- [x] 3.1 Build the per-package resolution screen listing each conflict's options with label, consequence, and size delta where applicable, verified by a pty run against a stub adapter showing options before commit
- [x] 3.2 Apply a chosen option to the marks/plan and re-resolve affected conflicts in the background without blocking the UI, verified by a unit test that selecting an option updates the plan and refreshes dependent conflicts

## 4. Plan gate

- [x] 4.1 Show the Yes/No popup when opening a plan with unresolved conflicts; [Yes] opens the resolver screen, verified by a pty run confirming [Yes] navigates to resolution
- [x] 4.2 On [No], render the plan with conflicting rows carrying the same indicator as the list, verified by a pty run showing the marked plan

## 5. Apply gating (non-conflicting subset)

- [x] 5.1 On apply, execute only non-conflicting operations and leave conflicting operations marked, recording a skip note, verified by a unit test that a mixed plan applies the independent ops and leaves the conflicted op marked
- [x] 5.2 Reconcile marks from post-run disk truth so skipped conflicting ops remain retryable, verified by a unit test asserting the skipped op is still marked after apply

## 6. Node dedupe/align flavor

- [x] 6.1 Surface redundant/duplicate Node copies across destinations on the per-destination table with versions and sizes, verified by a fixture test showing two destinations at different versions
- [x] 6.2 Offer align/consolidate/remove options with size deltas that mark the differing destination(s) for change, verified by a unit test that choosing align produces marks whose apply frees the reported space

## 7. Integration verification

- [x] 7.1 Run `go test ./...`, `./build.sh`, and pty runs of (a) the plan gate Yes/No flow and (b) a Node dedupe scenario, verifying all pass and matching the specs
