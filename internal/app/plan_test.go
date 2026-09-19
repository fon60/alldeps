package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"npmitude/internal/ecosystem"
	"npmitude/internal/domain"
)

func modelWithMarks(t *testing.T, prefixID string) Model {
	t.Helper()
	m := modelWithLoadedPrefix(t, prefixID, "alpha", "beta", "gamma", "delta")
	ps := m.state.Prefixes[prefixID]
	ps.Packages["alpha"].Mark = domain.MarkRemove
	for _, n := range []string{"beta", "delta"} {
		ps.Packages[n].InstalledVersion = ""
		ps.Packages[n].Origin = domain.OriginSearch
		ps.Packages[n].LatestVersion = "2.0.0"
		ps.Packages[n].Mark = domain.MarkInstall
	}
	ps.Packages["gamma"].LatestVersion = "2.0.0" // outdated 1.0.0 -> 2.0.0
	ps.Packages["gamma"].Mark = domain.MarkUpgrade
	return m
}

func TestBuildPlanGroups(t *testing.T) {
	m := modelWithMarks(t, "/p")
	groups := m.buildPlan()
	want := []struct {
		title string
		n     int
	}{
		{"Install", 2}, {"Remove", 1}, {"Upgrade", 1},
	}
	if len(groups) != len(want) {
		t.Fatalf("groups = %d, want %d", len(groups), len(want))
	}
	for i, g := range groups {
		if g.title != want[i].title || len(g.rows) != want[i].n {
			t.Fatalf("group %d = %s(%d), want %s(%d)", i, g.title, len(g.rows), want[i].title, want[i].n)
		}
	}
}

func TestBuildPlanExcludesHolds(t *testing.T) {
	m := modelWithMarks(t, "/p")
	m.state.SetMark("/p", "delta", domain.MarkHold)
	groups := m.buildPlan()
	for _, g := range groups {
		for _, r := range g.rows {
			if r.Name == "delta" {
				t.Fatalf("held delta appears in plan group %s", g.title)
			}
		}
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
	if got := m.state.PendingMarkCount("/p"); got != 4 {
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
	m.applyFrom = map[string]string{"gamma": "1.0.0"}

	// After the run: alpha removed OK, beta installed OK, gamma upgraded OK,
	// delta install failed (absent from disk), epsilon held throughout.
	fresh := map[string]*domain.PkgState{
		"beta":    {Name: "beta", InstalledVersion: "2.0.0", Origin: domain.OriginInstalled},
		"gamma":   {Name: "gamma", InstalledVersion: "2.0.0", Origin: domain.OriginInstalled},
		"epsilon": {Name: "epsilon", InstalledVersion: "1.0.0", Mark: domain.MarkHold, Origin: domain.OriginInstalled},
	}
	m = m.step(t, applyReloadMsg{prefixID: "/p", pkgs: fresh})

	if m.state.Applying {
		t.Fatal("Applying should be false after reload")
	}
	if !m.applyDone {
		t.Fatal("completion prompt should be showing")
	}
	ps := m.state.Prefixes["/p"]
	if got := ps.Packages["beta"].Mark; got != domain.MarkNone {
		t.Fatalf("beta mark = %v, want cleared (installed)", got)
	}
	if got := ps.Packages["gamma"].Mark; got != domain.MarkNone {
		t.Fatalf("gamma mark = %v, want cleared (upgraded)", got)
	}
	if _, ok := ps.Packages["alpha"]; ok {
		t.Fatal("alpha should be gone after successful removal")
	}
	if got := ps.Packages["delta"].Mark; got != domain.MarkInstall {
		t.Fatalf("delta mark = %v, want MarkInstall kept (failed install)", got)
	}
	if got := ps.Packages["epsilon"].Mark; got != domain.MarkHold {
		t.Fatalf("epsilon mark = %v, want MarkHold untouched", got)
	}
}

func TestApplyRefusedOnNonWritablePrefix(t *testing.T) {
	s := newStubEco()
	s.writable = false
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.eco = s
	m.state.Prefixes["/p"].Packages["alpha"].Mark = domain.MarkRemove
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
	m.applyBatches = []ecosystem.Batch{
		{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "pad-left", Version: "2.1.0"}}, Label: "install pad-left@2.1.0"},
		{Op: ecosystem.OpRemove, Items: []ecosystem.Item{{Name: "oldpkg"}}, Label: "remove oldpkg"},
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
	m.applyBatches = []ecosystem.Batch{
		{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "pad-left", Version: "2.1.0"}}, Label: "install pad-left@2.1.0"},
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

func TestCtrlCAbortsRunningBatch(t *testing.T) {
	m := modelWithMarks(t, "/p")
	ctx, cancel := context.WithCancel(context.Background())
	m.state.Applying = true
	m.applyBatches = []ecosystem.Batch{{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "x"}}, Label: "install x"}}
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
	m2.applyBatches = []ecosystem.Batch{{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "x"}}, Label: "install x"}}
	m2.applyBatchIdx = 0
	m2.applyCancel = cancel2
	m2 = m2.step(t, keyMsg(t, "j"))
	if ctx2.Err() != nil {
		t.Fatal("plain keys must not abort the running batch")
	}
}
