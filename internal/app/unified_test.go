package app

import (
	"testing"

	"npmitude/internal/domain"
	"npmitude/internal/ecosystem"
)

// twoDestModel builds a model with two loaded destinations of the stub
// manager; alpha is installed on both at 1.0.0.
func twoDestModel(t *testing.T) Model {
	t.Helper()
	m := modelWithLoadedPrefix(t, "/p1", "alpha")
	m.state.Prefixes["/p2"] = &domain.PrefixState{ID: "/p2", Packages: map[string]*domain.PkgState{
		"alpha": {Name: "alpha", InstalledVersion: "0.9.0", Origin: domain.OriginInstalled},
	}, Loaded: true}
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p1"}, {ID: "/p2"}}
	return m
}

func TestUnifiedListAggregatesByName(t *testing.T) {
	m := twoDestModel(t)
	rows := m.displayRows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (one row per name across destinations)", len(rows))
	}
	u := rows[0]
	if u.Name != "alpha" || !u.Installed() {
		t.Fatalf("row = %+v, want installed alpha", u)
	}
	if u.Headline == nil || u.Headline.InstalledVersion != "1.0.0" {
		t.Fatalf("headline = %+v, want the /p1 copy 1.0.0 (no ranks: id order)", u.Headline)
	}
	if u.Count != 1 {
		t.Fatalf("count = %d, want 1 additional destination", u.Count)
	}
}

func TestUnifiedListHeadlineByRank(t *testing.T) {
	m := twoDestModel(t)
	m.envsByManager["stub"] = []ecosystem.Environment{
		{ID: "/p1", Rank: "v18.0.0"},
		{ID: "/p2", Rank: "v24.0.0"},
	}
	rows := m.displayRows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	u := rows[0]
	if u.Headline == nil || u.Headline.InstalledVersion != "0.9.0" || u.HeadlineID != "/p2" {
		t.Fatalf("headline = %+v (%s), want the higher-ranked /p2 copy 0.9.0", u.Headline, u.HeadlineID)
	}
	if u.Count != 1 {
		t.Fatalf("count = %d, want 1 additional destination", u.Count)
	}
}

func TestInfoDestRemoveMarksOnlyThatDestination(t *testing.T) {
	m := twoDestModel(t)
	m = m.step(t, keyMsg(t, "enter")) // info tab for alpha
	if m.activeTab().Kind != TabInfo {
		t.Fatalf("active tab = %v, want info", m.activeTab().Kind)
	}
	// Cursor starts on the headline destination (/p1); move to /p2 and remove.
	m = m.step(t, keyMsg(t, "j"))
	if m.activeTab().DestCursor != 1 {
		t.Fatalf("dest cursor = %d, want 1 (/p2)", m.activeTab().DestCursor)
	}
	m = m.step(t, keyMsg(t, "-"))

	if got := m.state.Prefixes["/p2"].Packages["alpha"].MarkFor("stub"); got != domain.MarkRemove {
		t.Fatalf("/p2 mark = %v, want MarkRemove", got)
	}
	p1 := m.state.Prefixes["/p1"].Packages["alpha"]
	if got := p1.MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("/p1 mark = %v, want none (other destinations untouched)", got)
	}
	if p1.InstalledVersion != "1.0.0" {
		t.Fatal("/p1 installed state changed by a removal mark on /p2")
	}
}

func TestInfoDestInstallMarksOnlyThatDestination(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p1", "alpha")
	m.state.Prefixes["/p2"] = &domain.PrefixState{ID: "/p2", Packages: map[string]*domain.PkgState{}, Loaded: true}
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p1"}, {ID: "/p2"}}

	m = m.step(t, keyMsg(t, "enter")) // info for alpha (installed only on /p1)
	m = m.step(t, keyMsg(t, "j"))     // cursor to /p2 (absent there)
	m = m.step(t, keyMsg(t, "+"))

	p2 := m.state.Prefixes["/p2"].Packages["alpha"]
	if p2 == nil {
		t.Fatal("the install mark must create the package row on /p2")
	}
	if got := p2.MarkFor("stub"); got != domain.MarkInstall {
		t.Fatalf("/p2 mark = %v, want MarkInstall", got)
	}
	if got := m.state.Prefixes["/p1"].Packages["alpha"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("/p1 mark = %v, want none", got)
	}
}

func TestPlusSingleEligibleDestinationMarksDirectly(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p")
	m.state.Prefixes["/p"].Packages["ghost"] = &domain.PkgState{Name: "ghost", Origin: domain.OriginSearch, LatestVersion: "1.0.0"}

	m = m.step(t, keyMsg(t, "+"))

	if m.activeTab().Kind != TabList || m.overlay != OverlayNone {
		t.Fatalf("active tab/overlay = %v/%v, want list with no popup for a single eligible destination", m.activeTab().Kind, m.overlay)
	}
	if got := m.state.Prefixes["/p"].Packages["ghost"].MarkFor("stub"); got != domain.MarkInstall {
		t.Fatalf("mark = %v, want MarkInstall recorded directly", got)
	}
}

func TestPlusMultipleEligibleDestinationsOpensPopup(t *testing.T) {
	m := twoDestModel(t)
	ps1 := m.state.Prefixes["/p1"]
	ps1.Packages["ghost"] = &domain.PkgState{Name: "ghost", Origin: domain.OriginSearch, LatestVersion: "1.0.0"}

	m = m.step(t, keyMsg(t, "j")) // move the cursor to ghost (name-sorted after alpha)
	if m.selectedUnified() == nil || m.selectedUnified().Name != "ghost" {
		t.Fatalf("cursor not on ghost: %+v", m.selectedUnified())
	}
	m = m.step(t, keyMsg(t, "+"))

	if m.overlay != OverlayTargets {
		t.Fatalf("overlay = %v, want the install-target popup", m.overlay)
	}
	if len(m.targets) != 2 {
		t.Fatalf("eligible destinations = %d, want 2", len(m.targets))
	}
	m = m.step(t, keyMsg(t, "+")) // include the first destination
	m = m.step(t, keyMsg(t, "j"))
	m = m.step(t, keyMsg(t, "+")) // include the second as well
	m = m.step(t, keyMsg(t, "enter"))

	if m.overlay != OverlayNone {
		t.Fatalf("overlay after confirm = %v, want none (back to list)", m.overlay)
	}
	for _, d := range []string{"/p1", "/p2"} {
		p := m.state.Prefixes[d].Packages["ghost"]
		if p == nil || p.MarkFor("stub") != domain.MarkInstall {
			t.Fatalf("%s: ghost mark = %+v, want MarkInstall (one per chosen destination)", d, p)
		}
	}
}

func TestPlusPopupCancelRecordsNothing(t *testing.T) {
	m := twoDestModel(t)
	ps1 := m.state.Prefixes["/p1"]
	ps1.Packages["ghost"] = &domain.PkgState{Name: "ghost", Origin: domain.OriginSearch, LatestVersion: "1.0.0"}

	m = m.step(t, keyMsg(t, "j"))
	m = m.step(t, keyMsg(t, "+"))
	if m.overlay != OverlayTargets {
		t.Fatalf("overlay = %v, want the install-target popup", m.overlay)
	}
	m = m.step(t, keyMsg(t, "+")) // select something so a cancel is observable
	m = m.step(t, keyMsg(t, "esc"))

	if m.overlay != OverlayNone {
		t.Fatalf("overlay after cancel = %v, want none (back to list)", m.overlay)
	}
	for _, d := range []string{"/p1", "/p2"} {
		if p := m.state.Prefixes[d].Packages["ghost"]; p != nil && p.MarkFor("stub") == domain.MarkInstall {
			t.Fatalf("%s: cancel must not record the install mark", d)
		}
	}
}
