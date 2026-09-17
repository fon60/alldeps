package app

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"npmitude/internal/lock"
	"npmitude/internal/state"
)

func modelWithLoadedPrefix(t *testing.T, prefixID string, names ...string) Model {
	t.Helper()
	m := New()
	m.locks = lock.New(filepath.Join(t.TempDir(), "locks"))
	pkgs := map[string]*state.PkgState{}
	for _, n := range names {
		pkgs[n] = &state.PkgState{Name: n, InstalledVersion: "1.0.0", Origin: state.OriginInstalled}
	}
	m.state.Prefixes[prefixID] = &state.PrefixState{ID: prefixID, Packages: pkgs, Loaded: true}
	m.state.ActivePrefixID = prefixID
	return m
}

func TestSizesMsgFillsRow(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	_, _ = m.Update(sizesMsg{prefixID: "/p", name: "alpha", bytes: 1234})
	if got := m.state.Prefixes["/p"].Packages["alpha"].SizeBytes; got == nil || *got != 1234 {
		t.Fatalf("alpha SizeBytes = %v, want 1234", got)
	}
	if got := m.state.Prefixes["/p"].Packages["beta"].SizeBytes; got != nil {
		t.Fatalf("beta SizeBytes = %v, want nil", got)
	}

	_, _ = m.Update(sizesDoneMsg{prefixID: "/p"})
	if !m.state.Prefixes["/p"].SizesKnown {
		t.Fatal("SizesKnown should be true after sizesDoneMsg")
	}
}

func TestLoadPrefixMsgMergesAndKeepsMarks(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	m.state.SetMark("/p", "alpha", state.MarkRemove)
	m.state.Prefixes["/p"].Packages["alpha"].LatestVersion = "1.1.0"

	fresh := map[string]*state.PkgState{
		"alpha": {Name: "alpha", InstalledVersion: "1.0.0", Origin: state.OriginInstalled},
		// beta removed from disk, gamma newly installed
		"gamma": {Name: "gamma", InstalledVersion: "2.0.0", Origin: state.OriginInstalled},
	}
	_, _ = m.Update(loadPrefixMsg{prefixID: "/p", pkgs: fresh})

	ps := m.state.Prefixes["/p"]
	if got := ps.Packages["alpha"].Mark; got != state.MarkRemove {
		t.Fatalf("alpha mark = %v, want MarkRemove preserved", got)
	}
	if got := ps.Packages["alpha"].LatestVersion; got != "1.1.0" {
		t.Fatalf("alpha latest = %q, want cached 1.1.0", got)
	}
	if _, ok := ps.Packages["beta"]; ok {
		t.Fatal("beta should be gone after reload")
	}
	if _, ok := ps.Packages["gamma"]; !ok {
		t.Fatal("gamma should appear after reload")
	}
}

func TestActivePrefixMsgStartsLoad(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) // keep lock files out of the real state dir
	m := New()
	next, cmd := m.Update(activePrefixMsg{prefixID: "/p"})
	if next.(Model).state.ActivePrefixID != "/p" {
		t.Fatal("active prefix not set")
	}
	if cmd == nil {
		t.Fatal("expected load command after active prefix resolved")
	}
}

func TestQuitStillWorks(t *testing.T) {
	m := New()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}
