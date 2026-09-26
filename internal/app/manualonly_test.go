package app

import (
	"strings"
	"testing"
)

func TestManualOnlyToggleHidesAutomaticRows(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "direct", "trans")
	m.state.Prefixes["/p"].Packages["trans"].Automatic = true

	if got := names(m.visibleRows()); len(got) != 2 {
		t.Fatalf("default view must show all rows: %v", got)
	}

	m.tabs[0].Cursor = 1 // cursor sits on the automatic row
	m = m.step(t, keyMsg(t, "a"))
	if !m.manualOnly {
		t.Fatal("manualOnly must be on after a")
	}
	if got := names(m.visibleRows()); len(got) != 1 || got[0] != "direct" {
		t.Fatalf("manual-only view rows = %v, want only the direct package", got)
	}
	if m.tabs[0].Cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (clamped after the row under it vanished)", m.tabs[0].Cursor)
	}

	m = m.step(t, keyMsg(t, "a"))
	if m.manualOnly {
		t.Fatal("manualOnly must be off after a second toggle")
	}
	if got := names(m.visibleRows()); len(got) != 2 {
		t.Fatalf("rows after toggle-off = %v, want all two", got)
	}
}

func TestManualOnlyComposesWithFilter(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "direct", "trans")
	m.state.Prefixes["/p"].Packages["trans"].Automatic = true
	m.state.Prefixes["/p"].Packages["direct"].LatestVersion = "9.9.9" // upgradable

	m = m.step(t, keyMsg(t, "f"))
	m = typeRunes(m, t, "~u")
	m = m.step(t, keyMsg(t, "enter"))
	if got := names(m.visibleRows()); len(got) != 1 || got[0] != "direct" {
		t.Fatalf("~u rows = %v, want only the upgradable direct package", got)
	}

	// An upgradable automatic package passes the filter but the toggle hides it.
	m.state.Prefixes["/p"].Packages["trans"].LatestVersion = "9.9.9"
	if got := names(m.visibleRows()); len(got) != 2 {
		t.Fatalf("~u rows after trans becomes upgradable = %v, want both", got)
	}
	m = m.step(t, keyMsg(t, "a"))
	if got := names(m.visibleRows()); len(got) != 1 || got[0] != "direct" {
		t.Fatalf("rows with ~u and manual-only = %v, want the direct one only", got)
	}
}

func TestStatusLineShowsManualOnlyState(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	if out := render80x24(m); strings.Contains(out, "auto:hidden") {
		t.Fatalf("default view must not show the manual-only indicator:\n%s", out)
	}
	m = m.step(t, keyMsg(t, "a"))
	if out := render80x24(m); !strings.Contains(out, "auto:hidden") {
		t.Fatalf("status line must show the manual-only state:\n%s", out)
	}
}
