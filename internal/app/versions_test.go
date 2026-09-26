package app

import (
	"fmt"
	"strings"
	"testing"

	"npmitude/internal/domain"
	"npmitude/internal/ecosystem"
)

// versionsRowFor returns the ANSI-stripped data row of version v in a rendered
// versions screen (the version starts right after the 4-column flag cell).
func versionsRowFor(t *testing.T, out, v string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		s := ansiRE.ReplaceAllString(l, "")
		if len(s) > 4+len(v) && s[4:4+len(v)] == v {
			return s
		}
	}
	t.Fatalf("no versions row for %q in:\n%s", v, out)
	return ""
}

func TestVersionsColumnsInstalledInSeveralEnvs(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.envsByManager["stub"] = []ecosystem.Environment{
		{ID: "/p", Rank: "v20.0.0"},
		{ID: "/q", Rank: "v22.0.0"},
	}
	m.state.Prefixes["/q"] = &domain.PrefixState{
		ID:       "/q",
		Packages: map[string]*domain.PkgState{"alpha": {Name: "alpha", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled}},
		Loaded:   true,
	}
	m.state.Prefixes["/p"].Packages["alpha"].SizeBytes = ptr(int64(2048))

	doc := &ecosystem.Doc{
		Name:     "alpha",
		Versions: []string{"2.0.0", "1.5.0", "1.0.0"},
		Latest:   "2.0.0",
		UnpackedSizes: map[string]int64{
			"1.0.0": 9999, // must lose to the measured 2048
			"2.0.0": 48230,
		},
	}
	m.width, m.height = 80, 24
	m.openTab(TabVersions, "alpha")
	tabs := m.tabs
	idx := m.tabIdx
	tabs[idx].Name = "alpha"
	tabs[idx].Doc = doc
	m.positionVersionCursor(idx)

	whereW := 80 - colFlag - verColVersion - colSize
	out := render80x24(m)

	// Installed in two environments: state i, headline env v22.0.0 with a +1
	// presence counter, and the measured size wins over the unpacked one.
	wantInstalled := padRight("i  ", colFlag) +
		padRight("1.0.0", verColVersion) +
		padRight("2.0K", colSize) +
		padRight("v22.0.0+1", whereW)
	if got := versionsRowFor(t, out, "1.0.0"); got != wantInstalled {
		t.Errorf("installed row = %q, want %q", got, wantInstalled)
	}

	// Not installed anywhere: state p, absent indicator, registry unpacked size.
	wantLatest := padRight("p  ", colFlag) +
		padRight("2.0.0", verColVersion) +
		padRight("47.1K", colSize) +
		padRight("-", whereW)
	if got := versionsRowFor(t, out, "2.0.0"); got != wantLatest {
		t.Errorf("latest row = %q, want %q", got, wantLatest)
	}

	// No size source at all: the unknown indicator.
	wantUnknown := padRight("p  ", colFlag) +
		padRight("1.5.0", verColVersion) +
		padRight("…", colSize) +
		padRight("-", whereW)
	if got := versionsRowFor(t, out, "1.5.0"); got != wantUnknown {
		t.Errorf("unknown row = %q, want %q", got, wantUnknown)
	}
}

func TestVersionsFlagActionCharPerMarkShape(t *testing.T) {
	doc := &ecosystem.Doc{
		Name:     "alpha",
		Versions: []string{"2.0.0", "1.5.0", "1.2.3", "1.0.0"},
		Latest:   "2.0.0",
	}
	cases := []struct {
		name       string
		mutate     func(m *Model)
		wantTarget [][2]string // version -> flag
	}{
		{
			name: "install at pinned version",
			mutate: func(m *Model) {
				p := m.state.Prefixes["/p"].Packages["alpha"]
				p.InstalledVersion = ""
				p.Origin = domain.OriginSearch
				p.SetMarkEntry("stub", domain.MarkEntry{Mark: domain.MarkInstall, TargetVersion: "1.2.3"})
			},
			wantTarget: [][2]string{{"1.2.3", "p +"}},
		},
		{
			name: "upgrade at pinned version",
			mutate: func(m *Model) {
				m.state.Prefixes["/p"].Packages["alpha"].SetMarkEntry("stub", domain.MarkEntry{Mark: domain.MarkUpgrade, TargetVersion: "1.5.0"})
			},
			// 1.5.0 is not installed anywhere yet, so its state char stays p.
			wantTarget: [][2]string{{"1.5.0", "p u"}},
		},
		{
			name: "install unversioned targets the latest dist-tag",
			mutate: func(m *Model) {
				p := m.state.Prefixes["/p"].Packages["alpha"]
				p.InstalledVersion = ""
				p.Origin = domain.OriginSearch
				p.SetMarkEntry("stub", domain.MarkEntry{Mark: domain.MarkInstall})
			},
			wantTarget: [][2]string{{"2.0.0", "p +"}},
		},
		{
			name: "remove targets the active environment's installed version",
			mutate: func(m *Model) {
				m.state.Prefixes["/p"].Packages["alpha"].SetMarkEntry("stub", domain.MarkEntry{Mark: domain.MarkRemove})
			},
			wantTarget: [][2]string{{"1.0.0", "i -"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := modelWithLoadedPrefix(t, "/p", "alpha")
			tc.mutate(&m)
			m.openTab(TabVersions, "alpha")
			tabs := m.tabs
			idx := m.tabIdx
			tabs[idx].Name = "alpha"
			tabs[idx].Doc = doc

			out := render80x24(m)
			for _, want := range tc.wantTarget {
				got := versionsRowFor(t, out, want[0])
				if !strings.HasPrefix(got, want[1]) {
					t.Errorf("row %s flag = %q, want prefix %q", want[0], got[:4], want[1])
				}
			}
			// Every other row keeps the neutral action char.
			for _, v := range doc.Versions {
				targeted := false
				for _, want := range tc.wantTarget {
					if want[0] == v {
						targeted = true
						break
					}
				}
				if targeted {
					continue
				}
				got := versionsRowFor(t, out, v)
				if strings.Contains(got[:4], "+") || strings.Contains(got[:4], "-") || strings.Contains(got[:4], "u") {
					t.Errorf("row %s flag = %q, want a neutral action char", v, got[:4])
				}
			}
		})
	}
}

func TestVersionsViewportFollowsCursor(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	versions := make([]string, 30)
	for i := range versions {
		versions[i] = fmt.Sprintf("%d.0.0", 30-i) // index 0 is the newest
	}
	m.openTab(TabVersions, "alpha")
	tabs := m.tabs
	idx := m.tabIdx
	tabs[idx].Name = "alpha"
	tabs[idx].Doc = &ecosystem.Doc{Versions: versions}
	tabs[idx].VerCursor = 0

	m.width, m.height = 80, 24 // versions box holds 18 data rows

	for i := 0; i < 20; i++ {
		m = m.step(t, keyMsg(t, "j"))
	}
	if m.tabs[idx].VerCursor != 20 || m.tabs[idx].VerTop != 3 {
		t.Fatalf("after 20 downs: cursor=%d top=%d, want 20/3 (viewport must follow)", m.tabs[idx].VerCursor, m.tabs[idx].VerTop)
	}
	out := render80x24(m)
	if !strings.Contains(out, "10.0.0") || strings.Contains(out, "30.0.0") {
		t.Fatalf("cursor row 10.0.0 must be rendered and first row 30.0.0 scrolled out:\n%s", out)
	}

	m = m.step(t, keyMsg(t, "G"))
	if m.tabs[idx].VerCursor != 29 || m.tabs[idx].VerTop != 12 {
		t.Fatalf("after G: cursor=%d top=%d, want 29/12", m.tabs[idx].VerCursor, m.tabs[idx].VerTop)
	}
	out = render80x24(m)
	if !strings.Contains(out, "1.0.0") || strings.Contains(out, "19.0.0") {
		t.Fatalf("at the bottom the cursor row 1.0.0 must be the last data row (19.0.0 scrolled out):\n%s", out)
	}

	m = m.step(t, keyMsg(t, "g"))
	if m.tabs[idx].VerCursor != 0 || m.tabs[idx].VerTop != 0 {
		t.Fatalf("after g: cursor=%d top=%d, want 0/0", m.tabs[idx].VerCursor, m.tabs[idx].VerTop)
	}
	out = render80x24(m)
	if !strings.Contains(out, "30.0.0") || strings.Contains(out, "12.0.0") {
		t.Fatalf("at the top 30.0.0 must be visible and 12.0.0 below the fold:\n%s", out)
	}
}

func TestListVOpensVersionsTabAndDedupes(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.Prefixes["/p"].Packages["beta"] = &domain.PkgState{Name: "beta", Origin: domain.OriginSearch, LatestVersion: "3.0.0"}
	m.tabs[0].Cursor = 1 // highlight beta

	nextRaw, cmd := m.Update(keyMsg(t, "v"))
	if cmd == nil {
		t.Fatal("v on the list must start fetching the version history")
	}
	m = nextRaw.(Model)
	if m.activeTab().Kind != TabVersions || m.activeTab().Subject != "beta" {
		t.Fatalf("active tab = %v %q, want the versions tab for beta", m.activeTab().Kind, m.activeTab().Subject)
	}
	m = m.step(t, versionsMsg{name: "beta", doc: &ecosystem.Doc{Name: "beta", Versions: []string{"3.0.0", "2.0.0"}}})

	// Back on the list, v must focus the existing tab instead of duplicating it.
	m = m.step(t, keyMsg(t, "ctrl+h"))
	if m.activeTab().Kind != TabList {
		t.Fatalf("ctrl+h must return to the list, active = %v", m.activeTab().Kind)
	}
	m = m.step(t, keyMsg(t, "v"))
	if len(m.tabs) != 2 {
		t.Fatalf("second v must not duplicate the versions tab (tabs = %d)", len(m.tabs))
	}
	if m.activeTab().Kind != TabVersions || m.activeTab().Subject != "beta" {
		t.Fatalf("active tab = %v %q, want focus on the existing versions tab", m.activeTab().Kind, m.activeTab().Subject)
	}
	if m.activeTab().Doc == nil {
		t.Fatal("the focused versions tab must keep its loaded doc")
	}
}

func TestSearchVOpensVersionsTabForHighlightedHit(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabSearch, "gamma")
	m.applySearchResults("/p", "gamma", 0, []ecosystem.Hit{{Name: "gamma", Version: "1.0.0"}}, 0)

	m = m.step(t, keyMsg(t, "v"))
	if m.activeTab().Kind != TabVersions || m.activeTab().Subject != "gamma" {
		t.Fatalf("active tab = %v %q, want the versions tab for the highlighted search hit", m.activeTab().Kind, m.activeTab().Subject)
	}
}

func TestVersionsPlusShowsPinnedVersionInPlan(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.Prefixes["/p"].Packages["beta"] = &domain.PkgState{Name: "beta", Origin: domain.OriginSearch, LatestVersion: "3.0.0"}
	m.tabs[0].Cursor = 1 // highlight beta

	m = m.step(t, keyMsg(t, "v"))
	m = m.step(t, versionsMsg{name: "beta", doc: &ecosystem.Doc{
		Name:     "beta",
		Versions: []string{"3.0.0", "2.0.0", "1.0.0"},
		Latest:   "3.0.0",
	}})
	if v := m.activeTab().Doc.Versions[m.activeTab().VerCursor]; v != "3.0.0" {
		t.Fatalf("cursor must start on the latest version, got %q", v)
	}
	m = m.step(t, keyMsg(t, "j")) // 2.0.0
	m = m.step(t, keyMsg(t, "+"))

	b := m.state.Prefixes["/p"].Packages["beta"]
	if b.MarkFor("stub") != domain.MarkInstall || b.TargetVersionFor("stub") != "2.0.0" {
		t.Fatalf("+: mark=%v target=%q, want install at 2.0.0", b.MarkFor("stub"), b.TargetVersionFor("stub"))
	}

	m = m.step(t, keyMsg(t, "esc")) // close the versions tab back to the list
	if m.activeTab().Kind != TabList {
		t.Fatalf("active tab = %v, want the list", m.activeTab().Kind)
	}
	m = m.step(t, keyMsg(t, "g"))
	if m.activeTab().Kind != TabPlan {
		t.Fatalf("active tab = %v, want the plan", m.activeTab().Kind)
	}
	out := render80x24(m)
	if !strings.Contains(out, "beta@2.0.0") {
		t.Fatalf("plan must show the pinned version:\n%s", out)
	}
}

// versionsTabFor opens (or focuses) a versions tab for name with doc already
// loaded and the cursor positioned, returning its index.
func versionsTabFor(t *testing.T, m *Model, name string, doc *ecosystem.Doc) int {
	t.Helper()
	m.openTab(TabVersions, name)
	idx := m.tabIdx
	m.tabs[idx].Name = name
	m.tabs[idx].Doc = doc
	m.positionVersionCursor(idx)
	return idx
}

func TestVersionsPlusMarksInstallAtCursorVersion(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.Prefixes["/p"].Packages["beta"] = &domain.PkgState{Name: "beta", Origin: domain.OriginSearch, LatestVersion: "3.0.0"}
	versionsTabFor(t, &m, "beta", &ecosystem.Doc{Name: "beta", Versions: []string{"3.0.0", "2.0.0", "1.0.0"}, Latest: "3.0.0"})

	m = m.step(t, keyMsg(t, "j")) // cursor to 2.0.0
	m = m.step(t, keyMsg(t, "+"))

	b := m.state.Prefixes["/p"].Packages["beta"]
	if b.MarkFor("stub") != domain.MarkInstall || b.TargetVersionFor("stub") != "2.0.0" {
		t.Fatalf("mark=%v target=%q, want install at exactly 2.0.0", b.MarkFor("stub"), b.TargetVersionFor("stub"))
	}

	m = m.step(t, keyMsg(t, "+")) // repeat toggles the mark off
	if b.MarkFor("stub") != domain.MarkNone || b.TargetVersionFor("stub") != "" {
		t.Fatalf("after repeat +: mark=%v target=%q, want none", b.MarkFor("stub"), b.TargetVersionFor("stub"))
	}
}

func TestVersionsPlusOnInstalledManagesOnlyUpgradeMark(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha") // alpha installed at 1.0.0
	idx := versionsTabFor(t, &m, "alpha", &ecosystem.Doc{Name: "alpha", Versions: []string{"2.0.0", "1.5.0", "1.0.0"}, Latest: "2.0.0"})
	a := m.state.Prefixes["/p"].Packages["alpha"]

	// + on the exact installed version: no eligible environment, no mark at all
	// (in particular never a removal).
	if v := m.tabs[idx].Doc.Versions[m.tabs[idx].VerCursor]; v != "1.0.0" {
		t.Fatalf("cursor must start on the installed 1.0.0, got %q", v)
	}
	m = m.step(t, keyMsg(t, "+"))
	if a.HasMarks() {
		t.Fatalf("marks after + on the installed version = %+v, want none", a.Marks)
	}

	// + on 2.0.0: an upgrade mark pinned at 2.0.0; repeating it clears it.
	m = m.step(t, keyMsg(t, "k"))
	m = m.step(t, keyMsg(t, "k"))
	if v := m.tabs[idx].Doc.Versions[m.tabs[idx].VerCursor]; v != "2.0.0" {
		t.Fatalf("cursor version = %q, want 2.0.0", v)
	}
	m = m.step(t, keyMsg(t, "+"))
	if a.MarkFor("stub") != domain.MarkUpgrade || a.TargetVersionFor("stub") != "2.0.0" {
		t.Fatalf("mark=%v target=%q, want upgrade to 2.0.0", a.MarkFor("stub"), a.TargetVersionFor("stub"))
	}
	m = m.step(t, keyMsg(t, "+"))
	if a.MarkFor("stub") != domain.MarkNone {
		t.Fatalf("mark after repeat + = %v, want none (toggle off)", a.MarkFor("stub"))
	}
}

func TestVersionsMinusMarksRemovalToggle(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha") // alpha installed at 1.0.0
	idx := versionsTabFor(t, &m, "alpha", &ecosystem.Doc{Name: "alpha", Versions: []string{"2.0.0", "1.0.0"}, Latest: "2.0.0"})
	a := m.state.Prefixes["/p"].Packages["alpha"]

	m = m.step(t, keyMsg(t, "-"))
	if a.MarkFor("stub") != domain.MarkRemove {
		t.Fatalf("mark = %v, want MarkRemove", a.MarkFor("stub"))
	}
	m = m.step(t, keyMsg(t, "-")) // repeat cancels the removal
	if a.MarkFor("stub") != domain.MarkNone {
		t.Fatalf("mark after repeat - = %v, want none (toggle off)", a.MarkFor("stub"))
	}
	_ = idx
}

func TestVersionsMinusOnNotInstalledIsNoOpWithNotice(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.Prefixes["/p"].Packages["beta"] = &domain.PkgState{Name: "beta", Origin: domain.OriginSearch, LatestVersion: "3.0.0"}
	versionsTabFor(t, &m, "beta", &ecosystem.Doc{Name: "beta", Versions: []string{"3.0.0"}, Latest: "3.0.0"})

	m = m.step(t, keyMsg(t, "-"))

	b := m.state.Prefixes["/p"].Packages["beta"]
	if b.HasMarks() {
		t.Fatalf("marks after - on a not-installed row = %+v, want none (minus never installs)", b.Marks)
	}
	if m.notice == "" {
		t.Fatal("a no-op - must surface a notice")
	}
}

func TestVersionsPlusMultiEnvPopupDrivesPerEnvMarks(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p1")
	m.state.Prefixes["/p2"] = &domain.PrefixState{ID: "/p2", Packages: map[string]*domain.PkgState{}, Loaded: true}
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p1"}, {ID: "/p2"}}
	m.state.Prefixes["/p1"].Packages["ghost"] = &domain.PkgState{Name: "ghost", Origin: domain.OriginSearch, LatestVersion: "3.0.0"}

	idx := versionsTabFor(t, &m, "ghost", &ecosystem.Doc{Name: "ghost", Versions: []string{"3.0.0", "2.0.0"}, Latest: "3.0.0"})
	before := m.tabs[idx].VerCursor

	m = m.step(t, keyMsg(t, "+"))
	if m.overlay != OverlayTargets {
		t.Fatalf("overlay = %v, want the target popup for two eligible environments", m.overlay)
	}
	if len(m.targets) != 2 || m.targetsMode != targetsInstall || m.targetsVersion != "3.0.0" {
		t.Fatalf("popup = mode %d version %q over %d envs, want install@3.0.0 over 2", m.targetsMode, m.targetsVersion, len(m.targets))
	}

	m = m.step(t, keyMsg(t, "+")) // include /p1
	m = m.step(t, keyMsg(t, "j"))
	m = m.step(t, keyMsg(t, "-")) // exclude /p2 (stays out)
	m = m.step(t, keyMsg(t, "enter"))

	if m.overlay != OverlayNone {
		t.Fatalf("overlay after confirm = %v, want none", m.overlay)
	}
	if m.tabs[idx].VerCursor != before {
		t.Fatalf("versions cursor moved from %d to %d while the popup was open", before, m.tabs[idx].VerCursor)
	}
	p1 := m.state.Prefixes["/p1"].Packages["ghost"]
	if p1 == nil || p1.MarkFor("stub") != domain.MarkInstall || p1.TargetVersionFor("stub") != "3.0.0" {
		t.Fatalf("/p1 ghost = %+v, want install at 3.0.0", p1)
	}
	if p2 := m.state.Prefixes["/p2"].Packages["ghost"]; p2 != nil && p2.HasMarks() {
		t.Fatalf("/p2 ghost = %+v, want no mark (excluded environment)", p2)
	}
}

func TestVersionsMinusMultiEnvPopupMarksOnlySelected(t *testing.T) {
	m := twoDestModel(t) // alpha installed on /p1 (1.0.0) and /p2 (0.9.0)
	versionsTabFor(t, &m, "alpha", &ecosystem.Doc{Name: "alpha", Versions: []string{"1.0.0", "0.9.0"}, Latest: "1.0.0"})

	m = m.step(t, keyMsg(t, "-"))
	if m.overlay != OverlayTargets || m.targetsMode != targetsRemove {
		t.Fatalf("overlay/mode = %v/%d, want the remove-mode popup", m.overlay, m.targetsMode)
	}
	if len(m.targets) != 2 {
		t.Fatalf("eligible envs = %d, want 2 (installed in both)", len(m.targets))
	}

	m = m.step(t, keyMsg(t, "-")) // mark /p1 for removal
	m = m.step(t, keyMsg(t, "j"))
	m = m.step(t, keyMsg(t, "+")) // cancel /p2 (stays unmarked)
	m = m.step(t, keyMsg(t, "enter"))

	if got := m.state.Prefixes["/p1"].Packages["alpha"].MarkFor("stub"); got != domain.MarkRemove {
		t.Fatalf("/p1 mark = %v, want MarkRemove", got)
	}
	if got := m.state.Prefixes["/p2"].Packages["alpha"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("/p2 mark = %v, want none (not selected)", got)
	}
}

func TestVersionsPopupKeysDoNotReachVersionsScreen(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p1")
	m.state.Prefixes["/p2"] = &domain.PrefixState{ID: "/p2", Packages: map[string]*domain.PkgState{}, Loaded: true}
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p1"}, {ID: "/p2"}}
	m.state.Prefixes["/p1"].Packages["ghost"] = &domain.PkgState{Name: "ghost", Origin: domain.OriginSearch, LatestVersion: "3.0.0"}

	idx := versionsTabFor(t, &m, "ghost", &ecosystem.Doc{Name: "ghost", Versions: []string{"3.0.0", "2.0.0", "1.0.0"}, Latest: "3.0.0"})
	before := m.tabs[idx].VerCursor

	m = m.step(t, keyMsg(t, "+")) // opens the install popup
	if m.overlay != OverlayTargets {
		t.Fatalf("overlay = %v, want the target popup", m.overlay)
	}
	m = m.step(t, keyMsg(t, "j")) // moves the popup cursor, not the versions cursor
	m = m.step(t, keyMsg(t, "-")) // in install mode: exclude — must not create a removal mark
	m = m.step(t, keyMsg(t, "esc"))

	if m.overlay != OverlayNone {
		t.Fatalf("overlay after esc = %v, want none", m.overlay)
	}
	if m.tabs[idx].VerCursor != before {
		t.Fatalf("versions cursor moved from %d to %d while the popup was open", before, m.tabs[idx].VerCursor)
	}
	for _, d := range []string{"/p1", "/p2"} {
		if p := m.state.Prefixes[d].Packages["ghost"]; p != nil && p.MarkFor("stub") == domain.MarkRemove {
			t.Fatalf("%s: popup keys must never create a removal mark", d)
		}
	}
}
