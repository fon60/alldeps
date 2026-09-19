package app

import (
	"errors"
	"strings"
	"testing"

	"npmitude/internal/ecosystem"
	"npmitude/internal/domain"
)

func TestSearchMergeNoDuplicateInstalled(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.Prefixes["/p"].Packages["alpha"].LatestVersion = "1.0.0"

	hits := []ecosystem.Hit{
		{Name: "alpha", Version: "2.0.0", Description: "installed one"},
		{Name: "brand-new", Version: "3.0.0", Description: "fresh"},
	}
	m.applySearchResults("/p", "alpha", 0, hits, 0)

	ps := m.state.Prefixes["/p"]
	alpha := ps.Packages["alpha"]
	if alpha.Origin != domain.OriginInstalled {
		t.Fatalf("installed row must stay authoritative, origin = %v", alpha.Origin)
	}
	if alpha.InstalledVersion == "" {
		t.Fatal("installed version clobbered")
	}
	if len(ps.Packages) != 2 {
		t.Fatalf("package count = %d, want 2 (no duplicate for alpha)", len(ps.Packages))
	}
	fresh := ps.Packages["brand-new"]
	if fresh == nil || fresh.Origin != domain.OriginSearch || fresh.Flag() != "p*" {
		t.Fatalf("new search row wrong: %+v", fresh)
	}
	if fresh.LatestVersion != "3.0.0" || fresh.Description != "fresh" {
		t.Fatalf("search row fields wrong: %+v", fresh)
	}
	if !m.searchActive || m.searchQuery != "alpha" || !m.searchNames["alpha"] || !m.searchNames["brand-new"] {
		t.Fatalf("search view state wrong: active=%v query=%q names=%v", m.searchActive, m.searchQuery, m.searchNames)
	}
}

func TestSearchModeShowsOnlyResults(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	hits := []ecosystem.Hit{{Name: "gamma", Version: "1.0.0"}}
	m.applySearchResults("/p", "gamma", 0, hits, 0)

	rows := m.visibleRows()
	if len(rows) != 1 || rows[0].Name != "gamma" {
		names := []string{}
		for _, r := range rows {
			names = append(names, r.Name)
		}
		t.Fatalf("search mode must show only results, got %v", names)
	}
}

func TestSearchInstalledMatchShownAsInstalledRow(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	hits := []ecosystem.Hit{
		{Name: "alpha", Version: "9.9.9"},
		{Name: "gamma", Version: "1.0.0"},
	}
	m.applySearchResults("/p", "alpha", 0, hits, 0)

	rows := m.visibleRows()
	if len(rows) != 2 {
		t.Fatalf("expected installed match + new result, got %d rows", len(rows))
	}
	for _, r := range rows {
		if r.Name == "alpha" && r.Origin != domain.OriginInstalled {
			t.Fatal("installed match must render as the authoritative installed row")
		}
	}
	if len(m.state.Prefixes["/p"].Packages) != 2 {
		t.Fatal("duplicate row created for the installed package")
	}
}

func TestClearSearchRestoresInstalledList(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	hits := []ecosystem.Hit{
		{Name: "gamma", Version: "1.0.0"},
		{Name: "delta", Version: "2.0.0"},
	}
	m.applySearchResults("/p", "gamma", 0, hits, 0)
	m.state.SetMark("/p", "delta", domain.MarkInstall)

	m.clearSearch()

	if m.searchActive || m.searchQuery != "" || m.searchNames != nil {
		t.Fatal("search view state not reset")
	}
	ps := m.state.Prefixes["/p"]
	if _, ok := ps.Packages["gamma"]; ok {
		t.Fatal("unmarked search row must be dropped on clear")
	}
	if d := ps.Packages["delta"]; d == nil || d.Mark != domain.MarkInstall {
		t.Fatalf("marked search row must survive the clear: %+v", d)
	}
	rows := m.visibleRows()
	names := map[string]bool{}
	for _, r := range rows {
		names[r.Name] = true
	}
	if !names["alpha"] || !names["beta"] || names["gamma"] {
		t.Fatalf("installed list not restored, rows = %v", names)
	}
}

func TestEscKeyClearsSearch(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	hits := []ecosystem.Hit{{Name: "gamma", Version: "1.0.0"}}
	m.applySearchResults("/p", "gamma", 0, hits, 0)

	m = m.step(t, keyMsg(t, "esc"))

	if m.searchActive {
		t.Fatal("esc must clear the search view")
	}
	if _, ok := m.state.Prefixes["/p"].Packages["gamma"]; ok {
		t.Fatal("unmarked search row still present after esc")
	}
	if m.notice == "" {
		t.Fatal("expected a confirmation notice")
	}
}

func TestLocalMatchRefusedDuringSearch(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	hits := []ecosystem.Hit{{Name: "gamma", Version: "1.0.0"}}
	m.applySearchResults("/p", "gamma", 0, hits, 0)

	m = m.step(t, keyMsg(t, "l"))

	if m.prompt != nil {
		t.Fatal("local match prompt must not open while search results are shown")
	}
	if m.notice == "" {
		t.Fatal("expected a notice explaining the refusal")
	}
}

func TestSearchReplacesUnmarkedKeepsMarked(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p")
	ps := m.state.Prefixes["/p"]
	ps.Packages["foo-old"] = &domain.PkgState{Name: "foo-old", Origin: domain.OriginSearch, LatestVersion: "1.0.0"}
	marked := &domain.PkgState{Name: "foo-marked", Origin: domain.OriginSearch, LatestVersion: "2.0.0", Mark: domain.MarkInstall}
	ps.Packages["foo-marked"] = marked

	hits := []ecosystem.Hit{{Name: "bar-new", Version: "9.0.0"}}
	m.applySearchResults("/p", "bar", 0, hits, 0)

	if _, ok := ps.Packages["foo-old"]; ok {
		t.Fatal("unmarked search row should be replaced by the new search")
	}
	if got := ps.Packages["foo-marked"]; got == nil || got.Mark != domain.MarkInstall {
		t.Fatalf("marked search row must survive: %+v", got)
	}
	if _, ok := ps.Packages["bar-new"]; !ok {
		t.Fatal("new result missing")
	}
}

func TestSearchFailureLeavesStateUntouched(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.SetMark("/p", "alpha", domain.MarkRemove)
	m.state.FilterText = "~i"
	before := len(m.state.Prefixes["/p"].Packages)

	next, _ := m.Update(searchMsg{prefixID: "/p", query: "x", err: errors.New("network down")})
	m = next.(Model)

	if len(m.state.Prefixes["/p"].Packages) != before {
		t.Fatal("rows changed after failed search")
	}
	if m.state.Prefixes["/p"].Packages["alpha"].Mark != domain.MarkRemove {
		t.Fatal("mark lost after failed search")
	}
	if m.state.FilterText != "~i" {
		t.Fatal("filter changed after failed search")
	}
	if m.notice == "" {
		t.Fatal("expected an error notice")
	}
}

func TestSearchZeroMatchesNotices(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	next, _ := m.Update(searchMsg{prefixID: "/p", query: "zzz", hits: nil})
	m = next.(Model)
	if m.notice == "" {
		t.Fatal("expected a no-results notice")
	}
	if len(m.state.Prefixes["/p"].Packages) != 1 {
		t.Fatal("list state changed on zero matches")
	}
}

func TestSearchKeyOpensPrompt(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "/"))
	if m.prompt == nil || m.prompt.kind != PromptSearch {
		t.Fatal("search prompt should be open after /")
	}
}

func TestSearchPaginationLoadsNextPage(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	page1 := []ecosystem.Hit{
		{Name: "gamma", Version: "1.0.0"},
		{Name: "delta", Version: "2.0.0"},
	}
	m.applySearchResults("/p", "g", 0, page1, 4)

	if !m.hasMoreSearch() {
		t.Fatal("expected more pages after the first (fetched 2 of 4)")
	}
	if m.searchTotal != 4 || m.searchFetched != 2 {
		t.Fatalf("pagination state = %d/%d, want 2/4", m.searchFetched, m.searchTotal)
	}

	page2 := []ecosystem.Hit{
		{Name: "epsilon", Version: "3.0.0"},
		{Name: "zeta", Version: "4.0.0"},
	}
	m.applySearchResults("/p", "g", 2, page2, 4)

	if m.hasMoreSearch() {
		t.Fatal("no more pages expected after loading all 4")
	}
	names := map[string]bool{}
	for _, r := range m.visibleRows() {
		names[r.Name] = true
	}
	for _, want := range []string{"gamma", "delta", "epsilon", "zeta"} {
		if !names[want] {
			t.Fatalf("row %q missing after pagination, rows = %v", want, names)
		}
	}
	if names["alpha"] {
		t.Fatal("installed package must not appear in search results view")
	}
}

func TestSearchJAtEndTriggersNextPage(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	page1 := []ecosystem.Hit{
		{Name: "gamma", Version: "1.0.0"},
		{Name: "delta", Version: "2.0.0"},
	}
	m.applySearchResults("/p", "g", 0, page1, 4)

	// Move the cursor to the last visible row.
	for m.cursor < len(m.visibleRows())-1 {
		m = m.step(t, keyMsg(t, "j"))
	}
	if m.cursor != len(m.visibleRows())-1 {
		t.Fatalf("cursor = %d, want last row", m.cursor)
	}

	next, cmd := m.Update(keyMsg(t, "j"))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("j at the end of a paged search must start loading the next page")
	}
	if !m.searchLoading {
		t.Fatal("searchLoading must be set while the page fetch is in flight")
	}
	if m.cursor != len(m.visibleRows())-1 {
		t.Fatal("cursor must not move while waiting for the next page")
	}

	// A second j while loading must not double-fetch.
	if _, cmd2 := m.Update(keyMsg(t, "j")); cmd2 != nil {
		t.Fatal("second j while a page is in flight must not fetch again")
	}
}

func TestSearchPageFailureKeepsLoadedResults(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	page1 := []ecosystem.Hit{{Name: "gamma", Version: "1.0.0"}}
	m.applySearchResults("/p", "g", 0, page1, 4)
	m.searchLoading = true

	next, _ := m.Update(searchMsg{prefixID: "/p", query: "g", from: 1, err: errors.New("network down")})
	m = next.(Model)

	if m.searchLoading {
		t.Fatal("searchLoading must be cleared after a failed page fetch")
	}
	if !m.hasMoreSearch() {
		t.Fatal("user must be able to retry loading the next page")
	}
	if _, ok := m.state.Prefixes["/p"].Packages["gamma"]; !ok {
		t.Fatal("loaded results lost after a failed page fetch")
	}
	if m.notice == "" || !strings.Contains(m.notice, "more results") {
		t.Fatalf("expected a load-more failure notice, got %q", m.notice)
	}
}

func TestSearchStatusShowsLoadedOverTotal(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	page1 := []ecosystem.Hit{
		{Name: "gamma", Version: "1.0.0"},
		{Name: "delta", Version: "2.0.0"},
	}
	m.applySearchResults("/p", "g", 0, page1, 4)
	out := render80x24(m)
	if !strings.Contains(out, `search:"g" 2/4`) {
		t.Fatalf("status line must show loaded/total counts:\n%s", out)
	}
}

func TestSearchResultsKeepRegistryOrder(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	page1 := []ecosystem.Hit{
		{Name: "zeta", Version: "1.0.0"},
		{Name: "mid", Version: "2.0.0"},
		{Name: "alpha-x", Version: "3.0.0"},
	}
	m.applySearchResults("/p", "q", 0, page1, 5)

	got := make([]string, 0, len(m.visibleRows()))
	for _, r := range m.visibleRows() {
		got = append(got, r.Name)
	}
	want := []string{"zeta", "mid", "alpha-x"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("page 1 order = %v, want registry order %v (no sorting)", got, want)
	}

	page2 := []ecosystem.Hit{
		{Name: "omega", Version: "4.0.0"},
		{Name: "kilo", Version: "5.0.0"},
	}
	m.applySearchResults("/p", "q", 3, page2, 5)

	got = got[:0]
	for _, r := range m.visibleRows() {
		got = append(got, r.Name)
	}
	want = []string{"zeta", "mid", "alpha-x", "omega", "kilo"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("page 2 must append in registry order, got %v, want %v", got, want)
	}
}

func TestSearchStatusHidesLocalSort(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.applySearchResults("/p", "q", 0, []ecosystem.Hit{{Name: "zeta"}}, 1)
	out := render80x24(m)
	if strings.Contains(out, "sort:") {
		t.Fatalf("status line must not show the local sort during a search:\n%s", out)
	}
	m.clearSearch()
	if out := render80x24(m); !strings.Contains(out, "sort:name") {
		t.Fatalf("local sort must be shown again after clearing the search:\n%s", out)
	}
}
