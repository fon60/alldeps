package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"npmitude/internal/domain"
)

func keyMsg(t *testing.T, s string) tea.KeyMsg {
	t.Helper()
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// step drives one message through the model and returns the new model.
func (m Model) step(t *testing.T, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func typeRunes(m Model, t *testing.T, s string) Model {
	for _, r := range []rune(s) {
		m = m.step(t, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func TestFilterPromptValid(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	m.state.Prefixes["/p"].Packages["alpha"].LatestVersion = "9.9.9" // upgradable

	m = m.step(t, keyMsg(t, "f"))
	if m.prompt == nil {
		t.Fatal("prompt should be open after f")
	}
	m = typeRunes(m, t, "~u")
	m = m.step(t, keyMsg(t, "enter"))

	if m.state.FilterText != "~u" {
		t.Fatalf("FilterText = %q, want ~u", m.state.FilterText)
	}
	rows := m.visibleRows()
	if len(rows) != 1 || rows[0].Name != "alpha" {
		t.Fatalf("visible rows = %v, want only alpha (upgradable)", names(rows))
	}
}

func TestFilterPromptInvalidRetainsPrevious(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	m.state.Prefixes["/p"].Packages["alpha"].LatestVersion = "9.9.9"

	m = m.step(t, keyMsg(t, "f"))
	m = typeRunes(m, t, "~i")
	m = m.step(t, keyMsg(t, "enter"))
	if m.state.FilterText != "~i" {
		t.Fatalf("FilterText = %q, want ~i", m.state.FilterText)
	}

	// Submit an invalid expression: previous filter must be retained.
	m = m.step(t, keyMsg(t, "f"))
	m = typeRunes(m, t, "~z")
	m = m.step(t, keyMsg(t, "enter"))

	if m.state.FilterText != "~i" {
		t.Fatalf("FilterText = %q after invalid submit, want ~i retained", m.state.FilterText)
	}
	if m.notice == "" {
		t.Fatal("expected an error notice for the invalid expression")
	}
	if rows := m.visibleRows(); len(rows) != 2 {
		t.Fatalf("visible rows = %v, want both (filter ~i still active)", names(rows))
	}
}

func TestFilterPromptEscCancels(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "f"))
	m = typeRunes(m, t, "~i")
	m = m.step(t, keyMsg(t, "esc"))
	if m.prompt != nil {
		t.Fatal("prompt should be closed after esc")
	}
	if m.state.FilterText != "" {
		t.Fatalf("FilterText = %q, want empty (cancelled)", m.state.FilterText)
	}
}

func TestLocalMatchNarrowsInstalledOnly(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "npm", "opencode-ai")
	// a not-installed search row that also matches the pattern must be excluded
	m.state.Prefixes["/p"].Packages["npx-search"] = &domain.PkgState{
		Name: "npx-search", Origin: domain.OriginSearch, LatestVersion: "1.0.0",
	}

	m = m.step(t, keyMsg(t, "l"))
	if m.prompt == nil || m.prompt.kind != PromptLocal {
		t.Fatal("local prompt should be open after l")
	}
	m = typeRunes(m, t, "npm")

	rows := m.visibleRows()
	if got := names(rows); len(got) != 1 || got[0] != "npm" {
		t.Fatalf("local match rows = %v, want [npm] (installed only)", got)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (first match)", m.cursor)
	}

	m = m.step(t, keyMsg(t, "esc"))
	rows = m.visibleRows()
	if got := names(rows); len(got) != 3 {
		t.Fatalf("after cancel rows = %v, want all three restored", got)
	}
}

func TestLocalMatchEmptyPatternShowsAll(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	m = m.step(t, keyMsg(t, "l"))
	if rows := m.visibleRows(); len(rows) != 2 {
		t.Fatalf("empty local match should show all: %v", names(rows))
	}
}

func names(rows []*domain.PkgState) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

func TestCoalescedKeyBurstProcessedPerRune(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta", "gamma", "delta")
	// One KeyMsg carrying three runes (as bubbletea delivers a fast burst).
	m = m.step(t, keyMsg(t, "jjj"))
	if m.cursor != 3 {
		t.Fatalf("cursor = %d after coalesced jjj burst, want 3 (each rune must be processed)", m.cursor)
	}
}

func TestCoalescedBurstOpensPromptAndTypes(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "/pad"))
	if m.prompt == nil {
		t.Fatal("prompt should be open after coalesced /pad burst")
	}
	if got := m.prompt.input.Value(); got != "pad" {
		t.Fatalf("prompt value = %q, want pad", got)
	}
}
