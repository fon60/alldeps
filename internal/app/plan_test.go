package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"npmitude/internal/ecosystem"
	"npmitude/internal/domain"
	"npmitude/internal/lock"
)

func modelWithMarks(t *testing.T, prefixID string) Model {
	t.Helper()
	m := modelWithLoadedPrefix(t, prefixID, "alpha", "beta", "gamma", "delta")
	ps := m.state.Prefixes[prefixID]
	ps.Packages["alpha"].SetMarkFor("stub", domain.MarkRemove)
	for _, n := range []string{"beta", "delta"} {
		ps.Packages[n].InstalledVersion = ""
		ps.Packages[n].Origin = domain.OriginSearch
		ps.Packages[n].LatestVersion = "2.0.0"
		ps.Packages[n].SetMarkFor("stub", domain.MarkInstall)
	}
	ps.Packages["gamma"].LatestVersion = "2.0.0" // outdated 1.0.0 -> 2.0.0
	ps.Packages["gamma"].SetMarkFor("stub", domain.MarkUpgrade)
	return m
}

func TestBuildPlanGroups(t *testing.T) {
	m := modelWithMarks(t, "/p")
	groups := m.planGroups()
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1 (single destination)", len(groups))
	}
	g := groups[0]
	if g.dest != "/p" || g.manager != "stub" {
		t.Fatalf("group labeled %s/%s, want /p/stub", g.dest, g.manager)
	}
	if len(g.installs) != 2 || len(g.removals) != 1 || len(g.upgrades) != 1 {
		t.Fatalf("group rows = %d/%d/%d, want 2 installs, 1 removal, 1 upgrade", len(g.installs), len(g.removals), len(g.upgrades))
	}
	if g.invalid {
		t.Fatal("single-manager group must not be invalid")
	}
}

func TestBuildPlanExcludesHolds(t *testing.T) {
	m := modelWithMarks(t, "/p")
	m.state.SetMark("/p", "delta", "stub", domain.MarkHold)
	groups := m.planGroups()
	for _, g := range groups {
		for _, list := range [][]*domain.PkgState{g.installs, g.removals, g.upgrades} {
			for _, r := range list {
				if r.Name == "delta" {
					t.Fatal("held delta appears in the plan")
				}
			}
		}
	}
}

func TestPlanGroupsAcrossDestinations(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p1", "alpha")
	m.state.Prefixes["/p2"] = &domain.PrefixState{ID: "/p2", Packages: map[string]*domain.PkgState{
		"beta": {Name: "beta", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled},
	}, Loaded: true}
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p1"}, {ID: "/p2"}}
	m.state.Prefixes["/p1"].Packages["alpha"].SetMarkFor("stub", domain.MarkInstall)
	m.state.Prefixes["/p2"].Packages["beta"].SetMarkFor("stub", domain.MarkRemove)

	groups := m.planGroups()
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2 (one per destination)", len(groups))
	}
	want := []struct{ dest, manager string; n int }{
		{"/p1", "stub", 1}, {"/p2", "stub", 1},
	}
	for i, g := range groups {
		if g.dest != want[i].dest || g.manager != want[i].manager {
			t.Fatalf("group %d = %s/%s, want %s/%s (ordered by destination)", i, g.dest, g.manager, want[i].dest, want[i].manager)
		}
		if n := len(g.installs) + len(g.removals) + len(g.upgrades); n != want[i].n {
			t.Fatalf("group %d ops = %d, want %d", i, n, want[i].n)
		}
	}
	if groups[0].dest == "/p1" && len(groups[0].installs) != 1 {
		t.Fatal("/p1 group must carry the install")
	}
	if groups[1].dest == "/p2" && len(groups[1].removals) != 1 {
		t.Fatal("/p2 group must carry the removal")
	}
}

func TestApplyWithNoPendingShowsNotice(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "g"))
	if m.screen != ScreenList {
		t.Fatalf("screen = %v, want list", m.screen)
	}
	if m.notice == "" {
		t.Fatal("expected a notice about no pending changes")
	}
}

func TestPlanCancelLeavesMarks(t *testing.T) {
	m := modelWithMarks(t, "/p")
	m = m.step(t, keyMsg(t, "g"))
	if m.screen != ScreenPlan {
		t.Fatalf("screen = %v, want plan", m.screen)
	}
	m = m.step(t, keyMsg(t, "n"))
	if m.screen != ScreenList {
		t.Fatalf("screen after cancel = %v, want list", m.screen)
	}
	if got := m.state.PendingMarkCount("/p", "stub"); got != 4 {
		t.Fatalf("pending after cancel = %d, want 4 (marks untouched)", got)
	}
	if m.state.Applying {
		t.Fatal("cancel must not start an apply run")
	}
}

func TestApplySingleFlight(t *testing.T) {
	m := modelWithMarks(t, "/p")
	m = m.step(t, keyMsg(t, "g"))
	nextRaw, cmd := m.Update(keyMsg(t, "g"))
	next := nextRaw.(Model)
	if !next.state.Applying {
		t.Fatal("Applying should be true after confirm")
	}
	if cmd == nil {
		t.Fatal("confirm should return the first batch command")
	}
	// While the apply screen is up, keys are ignored (single flight: no second run).
	nextRaw2, cmd2 := next.Update(keyMsg(t, "g"))
	if cmd2 != nil {
		t.Fatal("a key during an in-progress apply must not start another run")
	}
	again := nextRaw2.(Model)
	if !again.state.Applying || again.applyDone {
		t.Fatal("apply state must be untouched by keys while running")
	}
}

func TestApplyReloadReconcilesMarks(t *testing.T) {
	m := modelWithMarks(t, "/p")
	m.state.Applying = true
	m.applyDests = []string{"/p"}
	m.applyFrom = map[string]string{"/p\x00gamma": "1.0.0"}

	// After the run: alpha removed OK, beta installed OK, gamma upgraded OK,
	// delta install failed (absent from disk), epsilon held throughout.
	fresh := map[string]*domain.PkgState{
		"beta":    {Name: "beta", InstalledVersion: "2.0.0", Origin: domain.OriginInstalled},
		"gamma":   {Name: "gamma", InstalledVersion: "2.0.0", Origin: domain.OriginInstalled},
		"epsilon": {Name: "epsilon", InstalledVersion: "1.0.0", Marks: map[string]domain.MarkEntry{"stub": {Mark: domain.MarkHold}}, Origin: domain.OriginInstalled},
	}
	m = m.step(t, applyReloadMsg{pkgsByDest: map[string]map[string]*domain.PkgState{"/p": fresh}})

	if m.state.Applying {
		t.Fatal("Applying should be false after reload")
	}
	if !m.applyDone {
		t.Fatal("completion prompt should be showing")
	}
	ps := m.state.Prefixes["/p"]
	if got := ps.Packages["beta"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("beta mark = %v, want cleared (installed)", got)
	}
	if got := ps.Packages["gamma"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("gamma mark = %v, want cleared (upgraded)", got)
	}
	if _, ok := ps.Packages["alpha"]; ok {
		t.Fatal("alpha should be gone after successful removal")
	}
	if got := ps.Packages["delta"].MarkFor("stub"); got != domain.MarkInstall {
		t.Fatalf("delta mark = %v, want MarkInstall kept (failed install)", got)
	}
	if got := ps.Packages["epsilon"].MarkFor("stub"); got != domain.MarkHold {
		t.Fatalf("epsilon mark = %v, want MarkHold untouched", got)
	}
}

func TestApplyRefusedOnNonWritablePrefix(t *testing.T) {
	s := newStubEco()
	s.writable = false
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.managers["stub"] = s
	m.state.Prefixes["/p"].Packages["alpha"].SetMarkFor("stub", domain.MarkRemove)
	m = m.step(t, keyMsg(t, "g"))
	nextRaw, _ := m.Update(keyMsg(t, "g"))
	next := nextRaw.(Model)

	if next.state.Applying {
		t.Fatal("apply must not start on a non-writable prefix")
	}
	if next.screen != ScreenPlan {
		t.Fatalf("screen = %v, want plan (still reviewing)", next.screen)
	}
	if next.notice == "" || !strings.Contains(next.notice, "not writable") {
		t.Fatalf("notice = %q, want an actionable writability message", next.notice)
	}
}

func TestApplyDonePromptReturns(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.applyDone = true
	m = m.step(t, keyMsg(t, "enter"))
	if m.applyDone {
		t.Fatal("Enter should dismiss the completion prompt")
	}
}

func TestApplyDonePromptQuits(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.applyDone = true
	_, cmd := m.Update(keyMsg(t, "q"))
	if cmd == nil {
		t.Fatal("q on the completion prompt should quit")
	}
}

func TestApplyScreenShowsRawOutput(t *testing.T) {
	m := modelWithMarks(t, "/p")
	m.state.Applying = true
	m.applyBatches = []applyBatch{
		{dest: "/p", manager: "stub", batch: ecosystem.Batch{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "pad-left", Version: "2.1.0"}}, Label: "install pad-left@2.1.0"}},
		{dest: "/p", manager: "stub", batch: ecosystem.Batch{Op: ecosystem.OpRemove, Items: []ecosystem.Item{{Name: "oldpkg"}}, Label: "remove oldpkg"}},
	}
	m.applyBatchIdx = 0
	m.applyCurrent = "install pad-left@2.1.0"
	m.appendApplyLog("install pad-left@2.1.0", "added 1 package, and audited 3 packages in 420ms\n", nil)

	out := render80x24(m)
	for _, want := range []string{
		"Applying changes —",
		"$ install pad-left@2.1.0",
		"added 1 package, and audited 3 packages in 420ms",
		"applying… step 1 of 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("apply screen missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[enter] continue") {
		t.Error("the continue prompt must not show while the run is in progress")
	}

	// All batches done, final reload in flight: no bogus "step 2 of 1".
	m.applyBatchIdx = len(m.applyBatches)
	if out := render80x24(m); !strings.Contains(out, "re-reading package list…") || strings.Contains(out, "step 2 of") {
		t.Fatalf("reload phase must show the re-reading line:\n%s", out)
	}
}

func TestApplyScreenCompletionPromptAndKeys(t *testing.T) {
	m := modelWithMarks(t, "/p")
	m.state.Applying = true
	m.applyBatches = []applyBatch{
		{dest: "/p", manager: "stub", batch: ecosystem.Batch{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "pad-left", Version: "2.1.0"}}, Label: "install pad-left@2.1.0"}},
	}
	m.applyBatchIdx = 1
	m.applyDone = true
	m.appendApplyLog("install pad-left@2.1.0", "added 1 package in 420ms\n", nil)

	out := render80x24(m)
	if !strings.Contains(out, "Apply finished —") || !strings.Contains(out, "[enter] continue    [q] quit") {
		t.Fatalf("completion screen missing title/prompt:\n%s", out)
	}

	next, _ := m.Update(keyMsg(t, "j")) // keys other than enter/q are ignored
	if next.(Model).applyDone != true {
		t.Fatal("stray keys must not dismiss the completion prompt")
	}
	next, _ = m.Update(keyMsg(t, "enter"))
	if next.(Model).applyDone {
		t.Fatal("enter must continue back to the list")
	}

	m2 := m
	nextRaw, cmd := m2.Update(keyMsg(t, "q"))
	_ = nextRaw
	if cmd == nil {
		t.Fatal("q on the completion screen must quit")
	}
}

func TestApplyLogFormatsEmptyAndErrorOutput(t *testing.T) {
	m := modelWithMarks(t, "/p")
	m.appendApplyLog("install silent", "\n", nil)
	m.appendApplyLog("remove broken", "npm ERR! bad thing\n", errors.New("exit status 1"))

	want := []string{
		"$ install silent",
		"    (no output)",
		"$ remove broken",
		"    npm ERR! bad thing",
		"    error: exit status 1",
	}
	if len(m.applyLog) != len(want) {
		t.Fatalf("log = %v, want %v", m.applyLog, want)
	}
	for i := range want {
		if m.applyLog[i] != want[i] {
			t.Fatalf("log[%d] = %q, want %q", i, m.applyLog[i], want[i])
		}
	}
}

// twoDestApplyModel builds a model with two loaded destinations of the stub
// manager and one pending mark on each (install on /p1, removal on /p2).
func twoDestApplyModel(t *testing.T) Model {
	t.Helper()
	m := modelWithLoadedPrefix(t, "/p1", "alpha")
	m.state.Prefixes["/p2"] = &domain.PrefixState{ID: "/p2", Packages: map[string]*domain.PkgState{
		"beta": {Name: "beta", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled},
	}, Loaded: true}
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p1"}, {ID: "/p2"}}
	m.state.Prefixes["/p1"].Packages["alpha"].SetMarkFor("stub", domain.MarkInstall)
	m.state.Prefixes["/p2"].Packages["beta"].SetMarkFor("stub", domain.MarkRemove)
	return m
}

func TestApplyLocksAllDestinationsBeforeFirstBatch(t *testing.T) {
	m := twoDestApplyModel(t)
	m = m.step(t, keyMsg(t, "g")) // plan preview
	nextRaw, cmd := m.Update(keyMsg(t, "g"))
	next := nextRaw.(Model)

	if !next.state.Applying {
		t.Fatal("Applying should be true after confirm")
	}
	if cmd == nil {
		t.Fatal("confirm should return the first batch command")
	}
	// Every touched destination must be locked before the first group runs.
	for _, d := range []string{"/p1", "/p2"} {
		if !next.locks.IsHeld(d) {
			t.Fatalf("destination %s is not locked before the first batch runs", d)
		}
	}

	// After the run finishes, the locks taken for it are released again.
	fresh := map[string]map[string]*domain.PkgState{
		"/p1": {"alpha": {Name: "alpha", InstalledVersion: "2.0.0", Origin: domain.OriginInstalled}},
		"/p2": {},
	}
	next = next.step(t, applyReloadMsg{pkgsByDest: fresh})
	for _, d := range []string{"/p1", "/p2"} {
		if next.locks.IsHeld(d) {
			t.Fatalf("destination %s is still locked after the run finished", d)
		}
	}
}

func TestApplySkipsInvalidGroups(t *testing.T) {
	m := twoDestApplyModel(t)
	yarn := newStubEco().withID("yarn")
	m.managers["yarn"] = yarn
	m.envsByManager["yarn"] = []ecosystem.Environment{{ID: "/p1"}}
	// /p1 is now marked under both stub and yarn -> invalid; /p2 stays valid.
	m.state.Prefixes["/p1"].Packages["alpha"].SetMarkFor("yarn", domain.MarkInstall)

	m = m.step(t, keyMsg(t, "g"))
	nextRaw, cmd := m.Update(keyMsg(t, "g"))
	next := nextRaw.(Model)

	if !next.state.Applying {
		t.Fatalf("apply must start for the valid groups (notice=%q)", next.notice)
	}
	if cmd == nil {
		t.Fatal("confirm should return the first batch command")
	}
	for _, ab := range next.applyBatches {
		if ab.dest == "/p1" {
			t.Fatalf("invalid group /p1 was queued for execution: %+v", ab)
		}
	}
	foundP2 := false
	for _, ab := range next.applyBatches {
		if ab.dest == "/p2" && ab.manager == "stub" {
			foundP2 = true
		}
	}
	if !foundP2 {
		t.Fatalf("valid group /p2 must still be queued: %+v", next.applyBatches)
	}
	log := strings.Join(next.applyLog, "\n")
	if !strings.Contains(log, "skipped /p1") || !strings.Contains(log, "two managers") {
		t.Fatalf("the skipped invalid group must be reported in the log:\n%s", log)
	}
}

func TestCtrlCAbortsRunningBatch(t *testing.T) {
	m := modelWithMarks(t, "/p")
	ctx, cancel := context.WithCancel(context.Background())
	m.state.Applying = true
	m.applyBatches = []applyBatch{{dest: "/p", manager: "stub", batch: ecosystem.Batch{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "x"}}, Label: "install x"}}}
	m.applyBatchIdx = 0
	m.applyCancel = cancel

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_ = next
	if cmd != nil {
		t.Fatal("ctrl+c must not start or continue any command")
	}
	if ctx.Err() == nil {
		t.Fatal("ctrl+c must cancel the running batch's context")
	}

	m2 := modelWithMarks(t, "/p")
	ctx2, cancel2 := context.WithCancel(context.Background())
	m2.state.Applying = true
	m2.applyBatches = []applyBatch{{dest: "/p", manager: "stub", batch: ecosystem.Batch{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "x"}}, Label: "install x"}}}
	m2.applyBatchIdx = 0
	m2.applyCancel = cancel2
	m2 = m2.step(t, keyMsg(t, "j"))
	if ctx2.Err() != nil {
		t.Fatal("plain keys must not abort the running batch")
	}
}

// twoGroupApplyModel builds a model whose plan has two groups: /p1 with an
// install (ghost) and a removal (alpha), /p2 with a removal (beta). The stub
// is wired so ListInstalled reports the given post-run disk state.
func twoGroupApplyModel(t *testing.T, s *stubEco) Model {
	t.Helper()
	m := New(s)
	m.locks = lock.New(filepath.Join(t.TempDir(), "locks"))
	m.state.Prefixes["/p1"] = &domain.PrefixState{ID: "/p1", Packages: map[string]*domain.PkgState{
		"alpha": {Name: "alpha", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled},
		"ghost": {Name: "ghost", Origin: domain.OriginSearch, LatestVersion: "2.0.0"},
	}, Loaded: true}
	m.state.Prefixes["/p2"] = &domain.PrefixState{ID: "/p2", Packages: map[string]*domain.PkgState{
		"beta": {Name: "beta", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled},
	}, Loaded: true}
	m.state.ActivePrefixID = "/p1"
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p1"}, {ID: "/p2"}}
	m.state.Prefixes["/p1"].Packages["ghost"].SetMarkFor("stub", domain.MarkInstall)
	m.state.SetMark("/p1", "alpha", "stub", domain.MarkRemove)
	m.state.SetMark("/p2", "beta", "stub", domain.MarkRemove)
	return m
}

// runApplyToDone drives a confirmed apply run by executing each returned
// command synchronously until the completion prompt is up.
func runApplyToDone(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for i := 0; i < 16 && !m.applyDone; i++ {
		if cmd == nil {
			t.Fatalf("apply in progress but no command at iteration %d", i)
		}
		nextRaw, nextCmd := m.Update(cmd())
		m = nextRaw.(Model)
		cmd = nextCmd
	}
	if !m.applyDone {
		t.Fatal("apply run did not reach the completion prompt")
	}
	return m
}

func executedLabels(s *stubEco) []string {
	out := make([]string, 0, len(s.executed))
	for _, b := range s.executed {
		out = append(out, b.Label)
	}
	return out
}

func TestApplyGroupFailureIsolatesGroups(t *testing.T) {
	s := newStubEco()
	// Post-run disk state: ghost install failed (absent), alpha removal never
	// ran (still installed), beta removed.
	s.packages = map[string][]ecosystem.Package{"/p1": {{Name: "alpha", Version: "1.0.0"}}}
	s.execFn = func(b ecosystem.Batch) (string, error) {
		if b.Op == ecosystem.OpInstall {
			return "npm ERR! network down\n", errors.New("exit status 1")
		}
		return "ok\n", nil
	}
	m := twoGroupApplyModel(t, s)

	m = m.step(t, keyMsg(t, "g"))
	nextRaw, cmd := m.Update(keyMsg(t, "g"))
	m = nextRaw.(Model)
	if !m.state.Applying {
		t.Fatalf("apply did not start (notice=%q)", m.notice)
	}
	m = runApplyToDone(t, m, cmd)

	// The failed group's second invocation must not have started; the other
	// group must still have run.
	if got := executedLabels(s); len(got) != 2 || !strings.Contains(got[0], "install ghost") || !strings.Contains(got[1], "remove beta") {
		t.Fatalf("executed batches = %v, want [install ghost, remove beta]", got)
	}
	if m.applyFailed != 1 {
		t.Fatalf("applyFailed = %d, want 1", m.applyFailed)
	}
	log := strings.Join(m.applyLog, "\n")
	for _, want := range []string{
		"group /p1 via stub: FAILED — remaining 1 invocation(s) not started",
		"group /p2 via stub: ok",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("apply log missing %q:\n%s", want, log)
		}
	}
	ps1 := m.state.Prefixes["/p1"]
	if got := ps1.Packages["ghost"].MarkFor("stub"); got != domain.MarkInstall {
		t.Fatalf("ghost mark = %v, want MarkInstall kept (install failed)", got)
	}
	if got := ps1.Packages["alpha"].MarkFor("stub"); got != domain.MarkRemove {
		t.Fatalf("alpha mark = %v, want MarkRemove kept (removal never ran)", got)
	}
	if _, ok := m.state.Prefixes["/p2"].Packages["beta"]; ok {
		t.Fatal("beta should be gone after successful removal")
	}
}

func TestApplyGroupFailureInLaterGroup(t *testing.T) {
	s := newStubEco()
	// Post-run disk state: ghost installed, beta removal failed (still there).
	s.packages = map[string][]ecosystem.Package{
		"/p1": {{Name: "ghost", Version: "2.0.0"}},
		"/p2": {{Name: "beta", Version: "1.0.0"}},
	}
	s.execFn = func(b ecosystem.Batch) (string, error) {
		if b.Op == ecosystem.OpRemove {
			return "npm ERR! EPERM\n", errors.New("exit status 1")
		}
		return "ok\n", nil
	}
	m := twoGroupApplyModel(t, s)
	m.state.Revert("/p1", "alpha", "stub") // /p1 keeps only its install: one batch per group

	m = m.step(t, keyMsg(t, "g"))
	nextRaw, cmd := m.Update(keyMsg(t, "g"))
	m = nextRaw.(Model)
	if !m.state.Applying {
		t.Fatalf("apply did not start (notice=%q)", m.notice)
	}
	m = runApplyToDone(t, m, cmd)

	// Both groups are attempted; the earlier group's success is reported.
	if got := executedLabels(s); len(got) != 2 || !strings.Contains(got[0], "install ghost") || !strings.Contains(got[1], "remove beta") {
		t.Fatalf("executed batches = %v, want both groups attempted", got)
	}
	log := strings.Join(m.applyLog, "\n")
	for _, want := range []string{"group /p1 via stub: ok", "group /p2 via stub: FAILED"} {
		if !strings.Contains(log, want) {
			t.Errorf("apply log missing %q:\n%s", want, log)
		}
	}
	if got := m.state.Prefixes["/p1"].Packages["ghost"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("ghost mark = %v, want cleared (installed on disk)", got)
	}
	if got := m.state.Prefixes["/p2"].Packages["beta"].MarkFor("stub"); got != domain.MarkRemove {
		t.Fatalf("beta mark = %v, want MarkRemove kept (removal failed)", got)
	}
}

func TestCtrlCAbortsRemainderOfPlan(t *testing.T) {
	s := newStubEco()
	m := twoGroupApplyModel(t, s)

	m = m.step(t, keyMsg(t, "g"))
	nextRaw, _ := m.Update(keyMsg(t, "g"))
	m = nextRaw.(Model)
	if !m.state.Applying {
		t.Fatalf("apply did not start (notice=%q)", m.notice)
	}
	if len(m.applyBatches) != 3 {
		t.Fatalf("batches = %d, want 3 (two in /p1, one in /p2)", len(m.applyBatches))
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.applyCancel = cancel
	m = m.step(t, tea.KeyMsg{Type: tea.KeyCtrlC})
	if ctx.Err() == nil {
		t.Fatal("ctrl+c must cancel the running batch's context")
	}

	// The aborted invocation reports its error; the whole plan remainder —
	// including other groups — is dropped.
	nextRaw, cmd := m.Update(applyBatchMsg{idx: 0, cmdLine: "install ghost@2.0.0 @ /p1", output: "", err: context.Canceled})
	m = nextRaw.(Model)
	if m.applyBatchIdx != len(m.applyBatches) {
		t.Fatalf("applyBatchIdx = %d, want %d (whole remainder dropped)", m.applyBatchIdx, len(m.applyBatches))
	}
	if cmd == nil {
		t.Fatal("the post-run reload command must still be offered after an abort")
	}
	nextRaw, _ = m.Update(cmd())
	m = nextRaw.(Model)
	if !m.applyDone {
		t.Fatal("the completion prompt must be offered so the user can read what happened")
	}
	log := strings.Join(m.applyLog, "\n")
	if !strings.Contains(log, "aborted by user — remaining invocations not started") {
		t.Fatalf("apply log missing the abort line:\n%s", log)
	}
}
