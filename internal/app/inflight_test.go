package app

import (
	"errors"
	"strings"
	"testing"

	"npmitude/internal/ecosystem"
)

// D6: an in-flight page/doc applies to its owning tab by (kind, subject) even
// while that tab is inactive; stale messages for closed or re-queried tabs
// are dropped.

func TestInfoDataLandsInInactiveTab(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "enter")) // info tab opens; doc fetch in flight
	if m.activeTab().Kind != TabInfo {
		t.Fatalf("active tab = %v, want info", m.activeTab().Kind)
	}
	m = m.step(t, keyMsg(t, "ctrl+h")) // move away before the doc arrives
	if m.activeTab().Kind != TabList {
		t.Fatalf("active tab = %v, want list", m.activeTab().Kind)
	}

	doc := &ecosystem.Doc{Name: "alpha", Description: "late doc"}
	m = m.step(t, infoDataMsg{name: "alpha", doc: doc})

	if m.activeTab().Kind != TabList {
		t.Fatalf("active tab = %v, want list (late data must not refocus)", m.activeTab().Kind)
	}
	i := m.findTab(TabInfo, "alpha")
	if i < 0 {
		t.Fatal("info tab vanished")
	}
	if m.tabs[i].Doc != doc {
		t.Fatalf("doc = %+v, want the late doc", m.tabs[i].Doc)
	}
}

func TestVersionsDataLandsInInactiveTab(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabInfo, "alpha")
	m.tabs[m.tabIdx].Name = "alpha"
	m = m.step(t, keyMsg(t, "v")) // versions tab opens; fetch in flight
	if m.activeTab().Kind != TabVersions {
		t.Fatalf("active tab = %v, want versions", m.activeTab().Kind)
	}
	m = m.step(t, keyMsg(t, "ctrl+h"))
	m = m.step(t, keyMsg(t, "ctrl+h")) // back to the list

	doc := &ecosystem.Doc{Name: "alpha", Versions: []string{"2.0.0", "1.0.0"}, Latest: "2.0.0"}
	m = m.step(t, versionsMsg{name: "alpha", doc: doc})

	if m.activeTab().Kind != TabList {
		t.Fatalf("active tab = %v, want list (late data must not refocus)", m.activeTab().Kind)
	}
	i := m.findTab(TabVersions, "alpha")
	if i < 0 {
		t.Fatal("versions tab vanished")
	}
	if m.tabs[i].Doc != doc {
		t.Fatalf("doc = %+v, want the late doc", m.tabs[i].Doc)
	}
}

func TestVersionsErrorLandsInInactiveTabWithoutRefocus(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabInfo, "alpha")
	m.tabs[m.tabIdx].Name = "alpha"
	m = m.step(t, keyMsg(t, "v"))
	m = m.step(t, keyMsg(t, "ctrl+h"))
	m = m.step(t, keyMsg(t, "ctrl+h"))

	m = m.step(t, versionsMsg{name: "alpha", err: errors.New("boom")})

	if m.activeTab().Kind != TabList {
		t.Fatalf("active tab = %v, want list (a failed fetch must not refocus)", m.activeTab().Kind)
	}
	i := m.findTab(TabVersions, "alpha")
	if i < 0 {
		t.Fatal("versions tab vanished")
	}
	if m.tabs[i].Err == "" {
		t.Fatal("the error must be recorded on the versions tab")
	}
}

func TestReadmeDataLandsInInactiveTab(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabInfo, "alpha")
	m.tabs[m.tabIdx].Name = "alpha"
	m = m.step(t, keyMsg(t, "C")) // readme tab opens; fetch in flight (no local readme)
	if m.activeTab().Kind != TabReadme {
		t.Fatalf("active tab = %v, want readme", m.activeTab().Kind)
	}
	m = m.step(t, keyMsg(t, "ctrl+h"))
	m = m.step(t, keyMsg(t, "ctrl+h")) // back to the list

	m = m.step(t, readmeMsg{name: "alpha", text: "# Alpha\nlate body", found: true})

	if m.activeTab().Kind != TabList {
		t.Fatalf("active tab = %v, want list (late data must not refocus)", m.activeTab().Kind)
	}
	i := m.findTab(TabReadme, "alpha")
	if i < 0 {
		t.Fatal("readme tab vanished")
	}
	if !strings.Contains(strings.Join(m.tabs[i].ReadmeLines, "\n"), "late body") {
		t.Fatalf("readme lines = %v, want the late text", m.tabs[i].ReadmeLines)
	}
}

func TestSearchPageLandsInInactiveTab(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabSearch, "rea") // page 0 in flight
	if m.activeTab().Kind != TabSearch {
		t.Fatalf("active tab = %v, want search", m.activeTab().Kind)
	}
	m = m.step(t, keyMsg(t, "ctrl+h")) // back to the list before the page arrives

	hits := []ecosystem.Hit{{Name: "react", Version: "18.0.0", Description: "a ui lib"}}
	m = m.step(t, searchMsg{prefixID: "/p", query: "rea", from: 0, hits: hits, total: 1})

	if m.activeTab().Kind != TabList {
		t.Fatalf("active tab = %v, want list (late data must not refocus)", m.activeTab().Kind)
	}
	i := m.findTab(TabSearch, "rea")
	if i < 0 {
		t.Fatal("search tab vanished")
	}
	if len(m.tabs[i].Order) != 1 || m.tabs[i].Order[0] != "react" {
		t.Fatalf("order = %v, want [react]", m.tabs[i].Order)
	}
}

func TestStaleMessagesForClosedTabsAreDropped(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "enter")) // info tab for alpha; doc in flight
	m = m.step(t, keyMsg(t, "esc"))   // close it before the doc arrives

	m = m.step(t, infoDataMsg{name: "alpha", doc: &ecosystem.Doc{Name: "alpha", Description: "stale"}})
	if i := m.findTab(TabInfo, "alpha"); i >= 0 {
		t.Fatal("a closed info tab must not be resurrected by a stale doc")
	}

	m.openTab(TabVersions, "alpha")
	m.tabs[m.tabIdx].Name = "alpha"
	m = m.step(t, keyMsg(t, "esc")) // close the versions tab
	m = m.step(t, versionsMsg{name: "alpha", doc: &ecosystem.Doc{Versions: []string{"1.0.0"}}})
	if i := m.findTab(TabVersions, "alpha"); i >= 0 {
		t.Fatal("a closed versions tab must not be resurrected by a stale doc")
	}

	m.openTab(TabReadme, "alpha")
	m.tabs[m.tabIdx].Name = "alpha"
	m = m.step(t, keyMsg(t, "esc")) // close the readme tab
	m = m.step(t, readmeMsg{name: "alpha", text: "stale", found: true})
	if i := m.findTab(TabReadme, "alpha"); i >= 0 {
		t.Fatal("a closed readme tab must not be resurrected by a stale page")
	}

	m.openTab(TabSearch, "rea")
	m = m.step(t, keyMsg(t, "q")) // close the search tab (back to list)
	if m.activeTab().Kind != TabList {
		t.Fatalf("active tab = %v, want list", m.activeTab().Kind)
	}
	before := len(m.state.Prefixes["/p"].Packages)
	m = m.step(t, searchMsg{prefixID: "/p", query: "rea", from: 0, hits: []ecosystem.Hit{{Name: "react", Version: "18.0.0"}}, total: 1})
	if got := len(m.state.Prefixes["/p"].Packages); got != before {
		t.Fatalf("packages = %d, want %d (a stale page must add nothing)", got, before)
	}
}

func TestStalePageForRequeriedSearchIsDropped(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabSearch, "rea") // page 0 for "rea" in flight
	i := m.findTab(TabSearch, "rea")
	if i < 0 {
		t.Fatal("search tab missing")
	}
	m.tabs[i].Subject = "newq" // the user re-queried in place

	m = m.step(t, searchMsg{prefixID: "/p", query: "rea", from: 0, hits: []ecosystem.Hit{{Name: "react", Version: "18.0.0"}}, total: 1})
	if i := m.findTab(TabSearch, "rea"); i >= 0 {
		t.Fatal("the re-queried tab must not accept a page for the old query")
	}
}
