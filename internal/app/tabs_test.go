package app

import (
	"testing"
)

func TestOpenTabAppendsAndActivates(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	idx, created := m.openTab(TabInfo, "alpha")
	if !created || idx != 1 {
		t.Fatalf("openTab = (%d,%v), want (1,true)", idx, created)
	}
	if len(m.tabs) != 2 || m.tabIdx != 1 {
		t.Fatalf("tabs = %d, tabIdx = %d, want the new tab appended and active", len(m.tabs), m.tabIdx)
	}
	if m.tabs[0].Kind != TabList || m.tabs[1].Kind != TabInfo || m.tabs[1].Subject != "alpha" {
		t.Fatalf("tabs = %+v, want [list info:alpha]", m.tabs)
	}
}

func TestOpenTabDedupesAnywhereInStrip(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabInfo, "alpha")    // idx 1
	m.openTab(TabVersions, "alpha") // idx 2

	// Re-opening the info tab from a non-adjacent position focuses the
	// existing one instead of appending a duplicate.
	idx, created := m.openTab(TabInfo, "alpha")
	if created || idx != 1 {
		t.Fatalf("re-open = (%d,%v), want (1,false) — dedupe anywhere in the strip", idx, created)
	}
	if m.tabIdx != 1 {
		t.Fatalf("tabIdx = %d, want focus on the existing info tab", m.tabIdx)
	}

	// Search tabs match by query.
	m.openTab(TabSearch, "alpha") // idx 3
	idx, created = m.openTab(TabSearch, "alpha")
	if created || idx != 3 {
		t.Fatalf("search re-open = (%d,%v), want (3,false)", idx, created)
	}

	// Help and Plan are singletons.
	m.openTab(TabHelp, "") // idx 4
	idx, created = m.openTab(TabHelp, "")
	if created || idx != 4 {
		t.Fatalf("help re-open = (%d,%v), want (4,false)", idx, created)
	}
	m.openTab(TabPlan, "") // idx 5
	idx, created = m.openTab(TabPlan, "")
	if created || idx != 5 {
		t.Fatalf("plan re-open = (%d,%v), want (5,false)", idx, created)
	}
	if len(m.tabs) != 6 {
		t.Fatalf("tabs = %d, want 6 (no duplicates appended)", len(m.tabs))
	}
}

func TestCloseActiveTabActivatesLeftNeighbor(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabInfo, "alpha") // idx 1
	m.openTab(TabHelp, "")      // idx 2

	m.closeActiveTab()
	if m.tabIdx != 1 || m.activeTab().Kind != TabInfo {
		t.Fatalf("after closing help: tabIdx = %d kind = %v, want the info tab", m.tabIdx, m.activeTab().Kind)
	}
	m.closeActiveTab()
	if m.tabIdx != 0 || m.activeTab().Kind != TabList {
		t.Fatalf("after closing info: tabIdx = %d kind = %v, want the root list", m.tabIdx, m.activeTab().Kind)
	}
	if len(m.tabs) != 1 {
		t.Fatalf("tabs = %d, want only the root left", len(m.tabs))
	}
}

func TestRootTabIsUncloseable(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.closeActiveTab()
	if len(m.tabs) != 1 || m.tabIdx != 0 || m.activeTab().Kind != TabList {
		t.Fatalf("root tab must survive closeActiveTab (tabs = %d, idx = %d)", len(m.tabs), m.tabIdx)
	}
}

func TestMoveTabKeysAndEdges(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabInfo, "alpha") // idx 1
	m.openTab(TabHelp, "")      // idx 2

	// Right edge: both right-move keys are inert.
	m = m.step(t, keyMsg(t, "ctrl+l"))
	if m.tabIdx != 2 {
		t.Fatal("ctrl+l at the right edge must be inert")
	}
	m = m.step(t, keyMsg(t, "ctrl+right"))
	if m.tabIdx != 2 {
		t.Fatal("ctrl+right at the right edge must be inert")
	}

	// Move left with ctrl+h, then with ctrl+left.
	m = m.step(t, keyMsg(t, "ctrl+h"))
	if m.tabIdx != 1 {
		t.Fatalf("ctrl+h = tab %d, want 1", m.tabIdx)
	}
	m = m.step(t, keyMsg(t, "ctrl+left"))
	if m.tabIdx != 0 {
		t.Fatalf("ctrl+left = tab %d, want 0", m.tabIdx)
	}

	// Left edge: both left-move keys are inert.
	m = m.step(t, keyMsg(t, "ctrl+h"))
	if m.tabIdx != 0 {
		t.Fatal("ctrl+h at the left edge must be inert")
	}
	m = m.step(t, keyMsg(t, "ctrl+left"))
	if m.tabIdx != 0 {
		t.Fatal("ctrl+left at the left edge must be inert")
	}
}

func TestPlainHLAreNotTabKeys(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabInfo, "alpha") // idx 1

	m = m.step(t, keyMsg(t, "h"))
	if m.tabIdx != 1 {
		t.Fatal("plain h must not move tabs")
	}
	m = m.step(t, keyMsg(t, "l"))
	if m.tabIdx != 1 || m.prompt != nil {
		t.Fatalf("plain l on the info tab must do nothing (tabIdx = %d, prompt = %v)", m.tabIdx, m.prompt)
	}

	// On a list-like tab plain l opens the local-match prompt instead of moving.
	m.closeActiveTab()
	m = m.step(t, keyMsg(t, "l"))
	if m.tabIdx != 0 || m.prompt == nil {
		t.Fatalf("plain l on the list tab must open the local prompt (tabIdx = %d, prompt = %v)", m.tabIdx, m.prompt)
	}
}

func TestMoveKeysInertUnderOverlayAndPrompt(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabInfo, "alpha") // idx 1

	m.overlay = OverlayPicker
	m = m.step(t, keyMsg(t, "ctrl+h"))
	if m.tabIdx != 1 {
		t.Fatal("ctrl+h under an overlay must be inert")
	}
	m.overlay = OverlayNone

	m.prompt = newPrompt(PromptFilter)
	m.prompt.input.Focus()
	m = m.step(t, keyMsg(t, "ctrl+l"))
	if m.tabIdx != 1 || m.prompt == nil {
		t.Fatalf("ctrl+l while a prompt has focus must be inert (tabIdx = %d)", m.tabIdx)
	}
}

func TestMoveKeysInertDuringApply(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabInfo, "alpha") // idx 1
	m.state.Applying = true

	m = m.step(t, keyMsg(t, "ctrl+h"))
	if m.tabIdx != 1 {
		t.Fatal("ctrl+h during an apply run must be inert")
	}
}

func TestQAndEscCloseNonRootTabsButQuitOnRoot(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.openTab(TabInfo, "alpha") // idx 1

	// q on a non-root tab closes the tab and returns no command.
	nextRaw, cmd := m.Update(keyMsg(t, "q"))
	m = nextRaw.(Model)
	if cmd != nil {
		t.Fatal("q on a non-root tab must close it, not quit")
	}
	if len(m.tabs) != 1 || m.tabIdx != 0 {
		t.Fatalf("after q: tabs = %d idx = %d, want the root list", len(m.tabs), m.tabIdx)
	}

	// esc on a non-root tab closes it as well.
	m.openTab(TabInfo, "alpha")
	m = m.step(t, keyMsg(t, "esc"))
	if len(m.tabs) != 1 || m.tabIdx != 0 {
		t.Fatalf("after esc: tabs = %d idx = %d, want the root list", len(m.tabs), m.tabIdx)
	}

	// q on the root tab with no pending marks quits.
	_, cmd = m.Update(keyMsg(t, "q"))
	if cmd == nil {
		t.Fatal("q on the root tab must quit")
	}
}
