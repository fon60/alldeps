package app

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"npmitude/internal/domain"
	"npmitude/internal/ecosystem"
	"npmitude/internal/lock"
)

func int64Ptr(v int64) *int64 { return &v }

// stepRefresh drives a refresh command returned by Update (possibly a batch
// of subcommands) and feeds every resulting message back through the model.
func (m Model) stepRefresh(t *testing.T, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = m.step(t, c())
		}
	default:
		m = m.step(t, msg)
	}
	return m
}

// modelWithConflict builds a single-destination model whose stub adapter
// reports one conflict for beta (marked for install) alongside an installed,
// unconflicted gamma. The conflict state is already resolved into the domain.
func modelWithConflict(t *testing.T) Model {
	t.Helper()
	s := newStubEco()
	s.conflictsFn = func(intent ecosystem.Intent) []ecosystem.Conflict {
		for _, it := range intent.Items {
			if it.Name == "beta" {
				return []ecosystem.Conflict{{Package: "beta", Message: "beta clashes with the rest of the tree", Options: []ecosystem.ResolutionOption{
					{Label: "downgrade beta to 1.9.4", Description: "another package drops to a major downgrade", SizeDelta: int64Ptr(-2 * 1024 * 1024), Effect: ecosystem.ResolutionEffect{Kind: "upgrade", Name: "beta", TargetVersion: "1.9.4"}},
					{Label: "skip installing beta", Description: "beta will not be installed", Effect: ecosystem.ResolutionEffect{Kind: "skip", Name: "beta"}},
				}}}
			}
		}
		return nil
	}
	m := New(s)
	m.locks = lock.New(filepath.Join(t.TempDir(), "locks"))
	sz := int64(5 * 1024 * 1024)
	m.state.Prefixes["/p"] = &domain.PrefixState{ID: "/p", Loaded: true, Packages: map[string]*domain.PkgState{
		"beta":  {Name: "beta", Origin: domain.OriginSearch, LatestVersion: "2.0.0"},
		"gamma": {Name: "gamma", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled, SizeBytes: &sz},
	}}
	m.state.ActivePrefixID = "/p"
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p"}}
	m.state.Prefixes["/p"].Packages["beta"].SetMarkFor("stub", domain.MarkInstall)

	cmd := m.maybeResolveConflicts()
	if cmd == nil {
		t.Fatal("expected a background resolve command for the pending marks")
	}
	return m.step(t, cmd())
}

// twoDestDedupeModel builds a model whose stub manager advertises HasDedupe
// with two loaded destinations carrying pad-left at different versions and
// measured sizes (10MB on /p1, 12MB on /p2).
func twoDestDedupeModel(t *testing.T) Model {
	t.Helper()
	m, _ := twoDestDedupeModelWithStub(t)
	return m
}

// Task 1.1: a stub adapter reporting a conflict surfaces it for the affected
// package, per (destination, package), and marks only that row on the list.
func TestStubReportedConflictSurfacesForPackage(t *testing.T) {
	m := modelWithConflict(t)

	if got := m.state.Prefixes["/p"].UnresolvedConflicts("beta"); len(got) != 1 || got[0].Package != "beta" {
		t.Fatalf("UnresolvedConflicts(beta) = %+v, want the reported conflict", got)
	}
	if got := m.state.Prefixes["/p"].UnresolvedConflicts("gamma"); len(got) != 0 {
		t.Fatalf("UnresolvedConflicts(gamma) = %+v, want none", got)
	}

	out := render80x24(m)
	// beta is a search-origin row (state p), so its conflicted flag cell is p*!.
	if !strings.Contains(out, "p*!") {
		t.Fatalf("conflicted row beta must carry the distinct conflict marker in its flag cell:\n%s", out)
	}
	if strings.Contains(out, "i*!") {
		t.Fatalf("unconflicted row gamma must not carry the conflict marker:\n%s", out)
	}
}

// Task 2.2: marking does not block navigation, filtering, sorting or marking
// other packages while a conflict is present.
func TestConflictDoesNotBlockOtherOperations(t *testing.T) {
	m := modelWithConflict(t)

	m = m.step(t, keyMsg(t, "j")) // navigate past the conflicted row
	if m.activeTab().Cursor != 1 {
		t.Fatalf("cursor = %d, want 1 (navigation must work)", m.activeTab().Cursor)
	}
	m = m.step(t, keyMsg(t, "S")) // cycle sort
	if m.state.SortKey != domain.SortVersion {
		t.Fatalf("sort key = %v, want version (sorting must work)", m.state.SortKey)
	}

	// the conflict filter keeps only the conflicted row
	m = m.step(t, keyMsg(t, "f"))
	m = typeRunes(m, t, "~c")
	m = m.step(t, keyMsg(t, "enter"))
	if rows := m.displayRows(); len(rows) != 1 || rows[0].Name != "beta" {
		t.Fatalf("~c filter shows %+v, want only beta", namesOf(rows))
	}
	m = m.step(t, keyMsg(t, "f"))
	m = m.step(t, keyMsg(t, "enter")) // clear the filter

	// mark another package normally
	m.tabs[0].Cursor = 0 // gamma (version sort puts it first)
	m = m.step(t, keyMsg(t, "-"))
	if got := m.state.Prefixes["/p"].Packages["gamma"].MarkFor("stub"); got != domain.MarkRemove {
		t.Fatalf("gamma mark = %v, want MarkRemove (marking others must work)", got)
	}
	if !m.cellConflicted("/p", "beta") {
		t.Fatal("the conflict must persist while other operations proceed")
	}
	if m.activeTab().Kind != TabList {
		t.Fatalf("active tab = %v, want list (nothing may force resolution on the user)", m.activeTab().Kind)
	}
}

// Task 3.1: the resolver screen lists each option with label, consequence and
// size delta before anything is committed.
func TestResolverShowsOptionsConsequencesAndDeltas(t *testing.T) {
	m := modelWithConflict(t)
	dest := m.conflictDestFor("beta")
	if dest != "/p" {
		t.Fatalf("conflictDestFor(beta) = %q, want /p", dest)
	}
	m, _ = m.openResolver(dest, "beta")

	out := render80x24(m)
	for _, want := range []string{
		"Resolve — beta",
		"beta clashes with the rest of the tree",
		"downgrade beta to 1.9.4",
		"another package drops to a major downgrade",
		"(frees 2.0M)",
		"skip installing beta",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("resolver screen missing %q:\n%s", want, out)
		}
	}
	if got := m.state.PendingMarkCount("/p", "stub"); got != 1 {
		t.Fatalf("pending marks = %d, want 1 (nothing committed before the choice)", got)
	}
	if m.state.Prefixes["/p"].ResolutionChosen("beta") {
		t.Fatal("no resolution pick may be recorded before the user commits")
	}
}

// Task 3.2: choosing an option updates the marks/plan and re-resolves the
// affected conflicts in the background (a command is returned, not inline).
func TestResolverChoiceUpdatesPlanAndReResolvesDependents(t *testing.T) {
	s := newStubEco()
	s.conflictsFn = func(intent ecosystem.Intent) []ecosystem.Conflict {
		has := map[string]bool{}
		for _, it := range intent.Items {
			has[it.Name] = true
		}
		var out []ecosystem.Conflict
		if has["beta"] {
			out = append(out, ecosystem.Conflict{Package: "beta", Message: "beta clash", Options: []ecosystem.ResolutionOption{
				{Label: "skip installing beta", Description: "beta will not be installed", Effect: ecosystem.ResolutionEffect{Kind: "skip", Name: "beta"}},
			}})
		}
		if has["beta"] && has["gamma"] { // dependent on the beta decision
			out = append(out, ecosystem.Conflict{Package: "gamma", Message: "gamma depends on the beta decision", Options: []ecosystem.ResolutionOption{
				{Label: "keep gamma", Description: "no change"},
			}})
		}
		return out
	}
	m := New(s)
	m.locks = lock.New(filepath.Join(t.TempDir(), "locks"))
	m.state.Prefixes["/p"] = &domain.PrefixState{ID: "/p", Loaded: true, Packages: map[string]*domain.PkgState{
		"beta":  {Name: "beta", Origin: domain.OriginSearch, LatestVersion: "2.0.0"},
		"gamma": {Name: "gamma", Origin: domain.OriginSearch, LatestVersion: "1.0.0"},
	}}
	m.state.ActivePrefixID = "/p"
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p"}}
	m.state.Prefixes["/p"].Packages["beta"].SetMarkFor("stub", domain.MarkInstall)
	m.state.Prefixes["/p"].Packages["gamma"].SetMarkFor("stub", domain.MarkInstall)

	m = m.stepRefresh(t, m.maybeResolveConflicts())
	if len(m.state.Prefixes["/p"].UnresolvedConflicts("beta")) != 1 {
		t.Fatal("beta conflict missing after the initial resolve")
	}
	if len(m.state.Prefixes["/p"].UnresolvedConflicts("gamma")) != 1 {
		t.Fatal("dependent gamma conflict missing after the initial resolve")
	}

	m, _ = m.openResolver("/p", "beta")
	nextRaw, cmd := m.Update(keyMsg(t, "enter")) // apply the option under the cursor
	m = nextRaw.(Model)

	if got := m.state.Prefixes["/p"].Packages["beta"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("beta mark = %v, want cleared by the skip option (plan updated)", got)
	}
	if cmd == nil {
		t.Fatal("choosing an option must trigger a background re-resolve")
	}
	m = m.stepRefresh(t, cmd)
	if got := m.state.Prefixes["/p"].Conflicts; len(got) != 0 {
		t.Fatalf("conflicts after re-resolve = %+v, want none (beta left the intent, gamma no longer dependent)", got)
	}
	if got := m.state.Prefixes["/p"].Packages["gamma"].MarkFor("stub"); got != domain.MarkInstall {
		t.Fatalf("gamma mark = %v, want untouched", got)
	}
}

// Task 4.1: opening a plan with unresolved conflicts raises the Yes/No popup;
// [Yes] opens the resolver screen for the affected package.
func TestPlanGateYesOpensResolver(t *testing.T) {
	m := modelWithConflict(t)
	m, _ = m.openPlan()
	if !m.activeTab().Gate {
		t.Fatal("the gate must be raised when the plan has unresolved conflicts")
	}
	out := render80x24(m)
	if !strings.Contains(out, "There are conflicts in the plan; would you like to resolve them?") {
		t.Fatalf("gate popup missing the question:\n%s", out)
	}

	m = m.step(t, keyMsg(t, "y"))
	rt := m.activeTab()
	if rt.Kind != TabResolver {
		t.Fatalf("active tab after [Yes] = %v, want resolver", rt.Kind)
	}
	if rt.RName != "beta" || rt.RDest != "/p" {
		t.Fatalf("resolver opened for %s/%s, want /p/beta", rt.RDest, rt.RName)
	}
	// The plan tab stays in the strip: closing the resolver returns to it with
	// the gate re-armed.
	m = m.step(t, keyMsg(t, "esc"))
	pt := m.activeTab()
	if pt.Kind != TabPlan || !pt.Gate {
		t.Fatalf("closing the resolver must return to the plan tab with the gate re-armed (kind=%v gate=%v)", pt.Kind, pt.Gate)
	}
}

// Task 4.2: [No] shows the plan with the conflicting rows carrying the same
// indicator as the list; no conflict raises no gate.
func TestPlanGateNoShowsMarkedPlan(t *testing.T) {
	m := modelWithConflict(t)
	m, _ = m.openPlan()
	m = m.step(t, keyMsg(t, "n"))
	if m.activeTab().Gate {
		t.Fatal("the gate must close on [No]")
	}
	if m.activeTab().Kind != TabPlan {
		t.Fatalf("active tab after [No] = %v, want plan", m.activeTab().Kind)
	}
	out := render80x24(m)
	if !strings.Contains(out, "install beta@2.0.0") {
		t.Fatalf("plan must list the conflicted install:\n%s", out)
	}
	if !strings.Contains(out, "! conflict") {
		t.Fatalf("conflicting plan row must carry the same indicator as the list:\n%s", out)
	}

	clean := modelWithConflict(t)
	clean.state.Prefixes["/p"].Conflicts = nil // no conflict at all
	clean, _ = clean.openPlan()
	if clean.activeTab().Gate {
		t.Fatal("the gate must not be raised without conflicts")
	}
}

// Task 5.1: applying a mixed plan executes only the non-conflicting
// operations; the conflicting one stays marked and a skip note is recorded.
func TestApplyExecutesNonConflictingSubset(t *testing.T) {
	s := newStubEco()
	s.conflictsFn = func(intent ecosystem.Intent) []ecosystem.Conflict {
		for _, it := range intent.Items {
			if it.Name == "beta" {
				return []ecosystem.Conflict{{Package: "beta", Message: "beta clash", Options: []ecosystem.ResolutionOption{
					{Label: "skip installing beta", Description: "beta will not be installed", Effect: ecosystem.ResolutionEffect{Kind: "skip", Name: "beta"}},
				}}}
			}
		}
		return nil
	}
	m := New(s)
	m.locks = lock.New(filepath.Join(t.TempDir(), "locks"))
	m.state.Prefixes["/p"] = &domain.PrefixState{ID: "/p", Loaded: true, Packages: map[string]*domain.PkgState{
		"alpha": {Name: "alpha", Origin: domain.OriginSearch, LatestVersion: "2.0.0"},
		"beta":  {Name: "beta", Origin: domain.OriginSearch, LatestVersion: "2.0.0"},
		"gamma": {Name: "gamma", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled},
	}}
	m.state.ActivePrefixID = "/p"
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p"}}
	m.state.Prefixes["/p"].Packages["alpha"].SetMarkFor("stub", domain.MarkInstall)
	m.state.Prefixes["/p"].Packages["beta"].SetMarkFor("stub", domain.MarkInstall)
	m.state.SetMark("/p", "gamma", "stub", domain.MarkRemove)

	m = m.stepRefresh(t, m.maybeResolveConflicts())
	if len(m.state.Prefixes["/p"].UnresolvedConflicts("beta")) != 1 {
		t.Fatal("beta conflict missing before apply")
	}

	m, _ = m.openPlan()
	m = m.step(t, keyMsg(t, "n")) // decline the gate, apply anyway
	nextRaw, cmd := m.Update(keyMsg(t, "g"))
	m = nextRaw.(Model)
	if !m.state.Applying {
		t.Fatalf("apply must start for the non-conflicting subset (notice=%q)", m.notice)
	}

	// Post-run disk truth: alpha installed, gamma removed, beta absent (skipped).
	s.packages = map[string][]ecosystem.Package{"/p": {{Name: "alpha", Version: "2.0.0"}}}
	m = runApplyToDone(t, m, cmd)

	labels := executedLabels(s)
	if len(labels) != 2 || !strings.Contains(labels[0], "install alpha") || !strings.Contains(labels[1], "remove gamma") {
		t.Fatalf("executed batches = %v, want [install alpha, remove gamma]", labels)
	}
	for _, l := range labels {
		if strings.Contains(l, "beta") {
			t.Fatalf("the conflicting install must not be executed: %v", labels)
		}
	}
	log := strings.Join(m.applyLog, "\n")
	if !strings.Contains(log, "skipped install beta on /p — unresolved conflict") {
		t.Fatalf("apply log must record the skipped conflicting operation:\n%s", log)
	}
	ps := m.state.Prefixes["/p"]
	if got := ps.Packages["beta"].MarkFor("stub"); got != domain.MarkInstall {
		t.Fatalf("beta mark = %v, want MarkInstall kept (conflicted op stays marked)", got)
	}
	if got := ps.Packages["alpha"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("alpha mark = %v, want cleared (executed and on disk)", got)
	}
	if _, ok := ps.Packages["gamma"]; ok {
		t.Fatal("gamma should be gone after the successful removal")
	}
}

// Task 5.2: reconciliation from post-run disk truth leaves a skipped
// conflicting op marked, so it stays retryable.
func TestSkippedConflictOpRemainsRetryable(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "beta")
	ps := m.state.Prefixes["/p"]
	ps.Packages["beta"].SetMarkFor("stub", domain.MarkRemove)
	ps.Conflicts = map[string][]ecosystem.Conflict{"beta": {{Package: "beta", Message: "clash"}}}

	// The run skipped beta (nothing executed): the post-run truth is unchanged.
	m.state.Applying = true
	m.applyDests = []string{"/p"}
	fresh := map[string]*domain.PkgState{"beta": {Name: "beta", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled}}
	m = m.step(t, applyReloadMsg{pkgsByDest: map[string]map[string]*domain.PkgState{"/p": fresh}})

	if got := m.state.Prefixes["/p"].Packages["beta"].MarkFor("stub"); got != domain.MarkRemove {
		t.Fatalf("skipped op mark = %v, want MarkRemove kept for a retry", got)
	}
	if !m.cellConflicted("/p", "beta") {
		t.Fatal("the conflict must remain unresolved so the retry path stays open")
	}
}

// Task 6.1: redundant Node copies across destinations surface as a conflict
// with both versions and sizes, on the per-destination data.
func TestDedupeSurfacesRedundantCopies(t *testing.T) {
	m := twoDestDedupeModel(t)

	cf := m.dedupeConflictFor("stub", "/p1", "pad-left")
	if cf == nil {
		t.Fatal("two destinations at different versions must derive a redundancy conflict")
	}
	if cf.Package != "pad-left" {
		t.Fatalf("conflict package = %q, want pad-left", cf.Package)
	}
	for _, want := range []string{"1.0.0 on /p1", "2.0.0 on /p2"} {
		if !strings.Contains(cf.Message, want) {
			t.Fatalf("conflict message = %q, want it to list %q", cf.Message, want)
		}
	}
	if len(cf.Options) != 3 {
		t.Fatalf("options = %d, want 3 (align, consolidate to the other dest, remove all): %+v", len(cf.Options), cf.Options)
	}

	// Both cells are conflicted; a single-copy package is not.
	if !m.cellConflicted("/p1", "pad-left") || !m.cellConflicted("/p2", "pad-left") {
		t.Fatal("both destinations holding the package must be conflicted")
	}
	m.state.Prefixes["/p1"].Packages["solo"] = &domain.PkgState{Name: "solo", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled}
	if m.cellConflicted("/p1", "solo") {
		t.Fatal("a package with a single copy must not be conflicted")
	}

	out := render80x24(m)
	if !strings.Contains(out, "i*!") {
		t.Fatalf("the unified row of the redundant package must carry the conflict marker:\n%s", out)
	}
}

// Task 6.2: choosing align marks the differing destination(s); applying the
// result removes exactly the copy whose size was reported as freed.
func TestDedupeAlignMarksAndApplyFreesReportedSpace(t *testing.T) {
	m, s := twoDestDedupeModelWithStub(t)

	cf := m.dedupeConflictFor("stub", "/p1", "pad-left")
	if cf == nil {
		t.Fatal("expected a redundancy conflict")
	}
	align := cf.Options[0]
	if !strings.HasPrefix(align.Label, "Align to 2.0.0") {
		t.Fatalf("first option = %q, want align to the highest version", align.Label)
	}
	if align.SizeDelta == nil || *align.SizeDelta != -10*1024*1024 {
		t.Fatalf("align size delta = %v, want -10MiB (the differing copy's measured size)", align.SizeDelta)
	}

	m.applyResolutionOption(align)
	if got := m.state.Prefixes["/p1"].Packages["pad-left"].MarkFor("stub"); got != domain.MarkRemove {
		t.Fatalf("/p1 mark = %v, want MarkRemove (the differing destination is marked)", got)
	}
	if got := m.state.Prefixes["/p2"].Packages["pad-left"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("/p2 mark = %v, want none (the kept copy is untouched)", got)
	}
	m = m.stepRefresh(t, m.maybeResolveConflicts()) // the choice's background re-resolve settles
	m2, _ := m.openPlan()
	if m2.activeTab().Gate {
		t.Fatal("a resolved cell must not re-raise the plan gate")
	}

	// Post-run disk truth: the 10MiB copy on /p1 is gone; /p2 keeps 2.0.0.
	s.packages = map[string][]ecosystem.Package{"/p2": {{Name: "pad-left", Version: "2.0.0"}}}
	nextRaw, cmd := m2.Update(keyMsg(t, "g")) // apply (no gate)
	m2 = nextRaw.(Model)
	if !m2.state.Applying {
		t.Fatalf("apply did not start (notice=%q)", m2.notice)
	}
	m2 = runApplyToDone(t, m2, cmd)

	for _, ab := range m2.applyBatches {
		if ab.dest != "/p1" {
			t.Fatalf("batch queued for %s, want only the differing destination /p1", ab.dest)
		}
	}
	labels := executedLabels(s)
	if len(labels) != 1 || !strings.Contains(labels[0], "remove pad-left") {
		t.Fatalf("executed batches = %v, want exactly [remove pad-left]", labels)
	}
	if _, ok := m2.state.Prefixes["/p1"].Packages["pad-left"]; ok {
		t.Fatal("the redundant copy must be gone from /p1 after apply")
	}
	if got := m2.state.Prefixes["/p2"].Packages["pad-left"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("/p2 mark = %v, want none", got)
	}
	if cf := m2.dedupeConflictFor("stub", "/p1", "pad-left"); cf != nil {
		t.Fatal("the redundancy must be gone after apply")
	}
}

// twoDestDedupeModelWithStub is twoDestDedupeModel plus the stub adapter for
// post-run disk state control.
func twoDestDedupeModelWithStub(t *testing.T) (Model, *stubEco) {
	t.Helper()
	s := newStubEco()
	caps := ecosystem.Caps{GlobalScope: true, HasDedupe: true}
	s.caps = &caps
	m := New(s)
	m.locks = lock.New(filepath.Join(t.TempDir(), "locks"))
	szA := int64(10 * 1024 * 1024)
	szB := int64(12 * 1024 * 1024)
	m.state.Prefixes["/p1"] = &domain.PrefixState{ID: "/p1", Loaded: true, Packages: map[string]*domain.PkgState{
		"pad-left": {Name: "pad-left", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled, SizeBytes: &szA},
	}}
	m.state.Prefixes["/p2"] = &domain.PrefixState{ID: "/p2", Loaded: true, Packages: map[string]*domain.PkgState{
		"pad-left": {Name: "pad-left", InstalledVersion: "2.0.0", Origin: domain.OriginInstalled, SizeBytes: &szB},
	}}
	m.state.ActivePrefixID = "/p1"
	m.envsByManager["stub"] = []ecosystem.Environment{
		{ID: "/p1", Rank: "v20.0.0"},
		{ID: "/p2", Rank: "v16.0.0"},
	}
	return m, s
}

func namesOf(rows []domain.UnifiedRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}
