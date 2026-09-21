package app

import (
	"path/filepath"
	"strings"
	"testing"

	"npmitude/internal/domain"
	"npmitude/internal/ecosystem"
	"npmitude/internal/lock"
)

type projSpec struct {
	manager string
	dest    string
	pkgs    []string
}

// newProjectModel builds a project-mode model where each spec's manager owns
// one destination with the given installed packages, preloaded (no discovery
// round trip). The first spec's manager is the active one.
func newProjectModel(t *testing.T, root string, specs ...projSpec) (Model, map[string]*stubEco) {
	t.Helper()
	managers := map[string]ecosystem.Ecosystem{}
	stubs := map[string]*stubEco{}
	for _, s := range specs {
		stub := newStubEco().withID(s.manager)
		stub.envs = []ecosystem.Environment{{ID: s.dest}}
		var list []ecosystem.Package
		for _, n := range s.pkgs {
			list = append(list, ecosystem.Package{Name: n, Version: "1.0.0"})
		}
		stub.packages = map[string][]ecosystem.Package{s.dest: list}
		managers[s.manager] = stub
		stubs[s.manager] = stub
	}
	ids := make([]string, 0, len(specs))
	for _, s := range specs {
		ids = append(ids, s.manager)
	}
	m := NewProject(root, managers, ids)
	m.locks = lock.New(filepath.Join(t.TempDir(), "locks"))
	t.Cleanup(m.releaseAll)
	for _, s := range specs {
		pkgs := map[string]*domain.PkgState{}
		for _, n := range s.pkgs {
			pkgs[n] = &domain.PkgState{Name: n, InstalledVersion: "1.0.0", Origin: domain.OriginInstalled}
		}
		m.state.Prefixes[s.dest] = &domain.PrefixState{ID: s.dest, Packages: pkgs, Loaded: true}
		m.envsByManager[s.manager] = []ecosystem.Environment{{ID: s.dest}}
	}
	if len(specs) > 0 {
		m.state.ActivePrefixID = specs[0].dest
	}
	return m, stubs
}

func rowNames(rows []domain.UnifiedRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

// switchManagerTo drives the M switcher to the named manager.
func (m Model) switchManagerTo(t *testing.T, want string) Model {
	t.Helper()
	m = m.step(t, keyMsg(t, "M"))
	for i, id := range m.managerIDs() {
		if id == want {
			for c := m.managerCursor; c != i; {
				if i < c {
					m = m.step(t, keyMsg(t, "k"))
				} else {
					m = m.step(t, keyMsg(t, "j"))
				}
				c = m.managerCursor
			}
			break
		}
	}
	m = m.step(t, keyMsg(t, "enter"))
	if m.activeManagerID != want {
		t.Fatalf("active manager = %q, want %q", m.activeManagerID, want)
	}
	return m
}

func TestProjectSwitcherExactlyOneActiveAndSwitches(t *testing.T) {
	root := t.TempDir()
	m, _ := newProjectModel(t, root,
		projSpec{"npm", "/p/node_modules", []string{"alpha"}},
		projSpec{"composer", "/p/vendor", []string{"beta"}})

	if m.mode != ModeProject {
		t.Fatalf("mode = %v, want ModeProject", m.mode)
	}
	if len(m.applicable) != 2 || m.applicable[0] != "npm" || m.applicable[1] != "composer" {
		t.Fatalf("applicable = %v, want [npm composer]", m.applicable)
	}

	m = m.step(t, keyMsg(t, "M"))
	if out := render80x24(m); strings.Count(out, "*") != 1 {
		t.Fatalf("switcher must mark exactly one adapter active:\n%s", out)
	}

	m = m.switchManagerTo(t, "composer")
	if m.overlay != OverlayNone {
		t.Fatalf("overlay after select = %v, want none (back to list)", m.overlay)
	}
	m = m.step(t, keyMsg(t, "M"))
	if out := render80x24(m); strings.Count(out, "*") != 1 {
		t.Fatalf("after switching, still exactly one adapter active:\n%s", out)
	}
}

func TestProjectListScopedToActiveAdapter(t *testing.T) {
	root := t.TempDir()
	m, _ := newProjectModel(t, root,
		projSpec{"npm", "/p/node_modules", []string{"alpha", "helper"}},
		projSpec{"composer", "/p/vendor", []string{"beta", "helper"}})

	if got := rowNames(m.displayRows()); len(got) != 2 || got[0] != "alpha" || got[1] != "helper" {
		t.Fatalf("npm rows = %v, want only the npm adapter's packages [alpha helper]", got)
	}

	m = m.switchManagerTo(t, "composer")
	if got := rowNames(m.displayRows()); len(got) != 2 || got[0] != "beta" || got[1] != "helper" {
		t.Fatalf("composer rows = %v, want only the composer adapter's packages [beta helper]", got)
	}
	for _, n := range rowNames(m.displayRows()) {
		if n == "alpha" {
			t.Fatal("alpha (an npm-only package) leaked into the composer view")
		}
	}
}

func TestProjectSwitchPreservesMarksAndInstalledState(t *testing.T) {
	root := t.TempDir()
	m, stubs := newProjectModel(t, root,
		projSpec{"npm", "/p/node_modules", []string{"alpha"}},
		projSpec{"composer", "/p/vendor", []string{"beta"}})
	psNpm := m.state.Prefixes["/p/node_modules"]
	psNpm.Packages["alpha"].SetMarkFor("npm", domain.MarkInstall)

	m = m.switchManagerTo(t, "composer")
	if calls := stubs["npm"].listCalls; calls != 0 {
		t.Fatalf("switching away re-fetched npm state (%d list calls), want 0", calls)
	}
	if got := psNpm.Packages["alpha"].MarkFor("npm"); got != domain.MarkInstall {
		t.Fatalf("npm mark after switching away = %v, want MarkInstall (marks survive)", got)
	}

	m = m.switchManagerTo(t, "npm")
	if calls := stubs["composer"].listCalls; calls != 0 {
		t.Fatalf("switching back re-fetched composer state (%d list calls), want 0", calls)
	}
	if got := psNpm.Packages["alpha"].MarkFor("npm"); got != domain.MarkInstall {
		t.Fatalf("npm mark after round trip = %v, want MarkInstall", got)
	}
	if !psNpm.Loaded || psNpm.Packages["alpha"].InstalledVersion != "1.0.0" {
		t.Fatal("installed state changed by the adapter switch")
	}
	if m.activeManagerID != "npm" {
		t.Fatalf("active manager = %q, want npm", m.activeManagerID)
	}
}

func TestProjectSameNameAcrossAdaptersNotConflated(t *testing.T) {
	root := t.TempDir()
	m, _ := newProjectModel(t, root,
		projSpec{"npm", "/p/node_modules", []string{"helper"}},
		projSpec{"composer", "/p/vendor", []string{"helper"}})

	m = m.step(t, keyMsg(t, "-")) // mark helper for removal under npm

	npmPS := m.state.Prefixes["/p/node_modules"]
	compPS := m.state.Prefixes["/p/vendor"]
	if got := npmPS.Packages["helper"].MarkFor("npm"); got != domain.MarkRemove {
		t.Fatalf("npm helper mark = %v, want MarkRemove", got)
	}
	if compPS.Packages["helper"].HasMarks() {
		t.Fatalf("composer helper must stay unmarked, got marks %v", compPS.Packages["helper"].Marks)
	}

	m = m.switchManagerTo(t, "composer")
	rows := m.displayRows()
	if len(rows) != 1 || rows[0].Name != "helper" {
		t.Fatalf("composer rows = %+v, want the single helper row", rows)
	}
	if got := rows[0].Flag("composer"); got != "i*" {
		t.Fatalf("composer helper flag = %q, want i* (no mark carried over)", got)
	}
	if got := npmPS.Packages["helper"].MarkFor("npm"); got != domain.MarkRemove {
		t.Fatalf("npm helper mark after switch = %v, want MarkRemove kept", got)
	}
}

func TestProjectPlanScopedToActiveAdapter(t *testing.T) {
	root := t.TempDir()
	m, stubs := newProjectModel(t, root,
		projSpec{"npm", "/p/node_modules", []string{"helper"}},
		projSpec{"composer", "/p/vendor", []string{"helper"}})

	// Pending npm removal must not leak into a plan built under composer.
	m.state.SetMark("/p/node_modules", "helper", "npm", domain.MarkRemove)

	m = m.switchManagerTo(t, "composer")
	m = m.step(t, keyMsg(t, "-")) // mark helper for removal under composer

	groups := m.planGroups()
	if len(groups) != 1 {
		t.Fatalf("plan groups = %d, want 1 (the active adapter only)", len(groups))
	}
	if groups[0].manager != "composer" || groups[0].dest != "/p/vendor" {
		t.Fatalf("group = %s/%s, want composer//p/vendor", groups[0].manager, groups[0].dest)
	}

	m = m.step(t, keyMsg(t, "g")) // open the plan tab
	if m.activeTab().Kind != TabPlan {
		t.Fatalf("active tab = %v, want plan", m.activeTab().Kind)
	}
	if out := render80x24(m); strings.Contains(out, "npm @") {
		t.Fatalf("plan built under composer must not show npm operations:\n%s", out)
	}

	nextRaw, cmd := m.Update(keyMsg(t, "g")) // apply
	m = nextRaw.(Model)
	if !m.state.Applying || cmd == nil {
		t.Fatal("apply must start with the first batch command")
	}
	batchMsg := cmd() // runs the composer batch; the stub records it
	nextRaw, reloadCmd := m.Update(batchMsg)
	m = nextRaw.(Model)
	if reloadCmd == nil {
		t.Fatal("the finished batch must schedule the post-run re-read")
	}
	reloadMsg := reloadCmd() // re-reads only the composer destination
	m = m.step(t, reloadMsg)

	if n := len(stubs["composer"].executed); n != 1 {
		t.Fatalf("composer executed %d batch(es), want 1", n)
	}
	b := stubs["composer"].executed[0]
	if b.Op != ecosystem.OpRemove || len(b.Items) != 1 || b.Items[0].Name != "helper" {
		t.Fatalf("composer batch = %+v, want a single helper removal", b)
	}
	if n := len(stubs["npm"].executed); n != 0 {
		t.Fatalf("npm executed %d batch(es), want 0 (plan is scoped to composer)", n)
	}
	npmPS := m.state.Prefixes["/p/node_modules"]
	if got := npmPS.Packages["helper"].MarkFor("npm"); got != domain.MarkRemove {
		t.Fatalf("npm helper mark = %v, want MarkRemove still pending (not executed)", got)
	}
	if !npmPS.Packages["helper"].Installed() {
		t.Fatal("npm collection was touched by the composer apply")
	}
}
