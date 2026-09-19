package app

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"npmitude/internal/domain"
	"npmitude/internal/ecosystem"
	"npmitude/internal/lock"
)

func modelWithLoadedPrefix(t *testing.T, prefixID string, names ...string) Model {
	t.Helper()
	m := New(newStubEco())
	m.locks = lock.New(filepath.Join(t.TempDir(), "locks"))
	pkgs := map[string]*domain.PkgState{}
	for _, n := range names {
		pkgs[n] = &domain.PkgState{Name: n, InstalledVersion: "1.0.0", Origin: domain.OriginInstalled}
	}
	m.state.Prefixes[prefixID] = &domain.PrefixState{ID: prefixID, Packages: pkgs, Loaded: true}
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
	m.state.SetMark("/p", "alpha", domain.MarkRemove)
	m.state.Prefixes["/p"].Packages["alpha"].LatestVersion = "1.1.0"

	fresh := map[string]*domain.PkgState{
		"alpha": {Name: "alpha", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled},
		// beta removed from disk, gamma newly installed
		"gamma": {Name: "gamma", InstalledVersion: "2.0.0", Origin: domain.OriginInstalled},
	}
	_, _ = m.Update(loadEnvMsg{prefixID: "/p", pkgs: fresh})

	ps := m.state.Prefixes["/p"]
	if got := ps.Packages["alpha"].Mark; got != domain.MarkRemove {
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

func TestDiscoverMsgStartsLoad(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) // keep lock files out of the real state dir
	m := New(newStubEco())
	next, cmd := m.Update(discoverMsg{envs: []ecosystem.Environment{{ID: "/p", Meta: ecosystem.Meta{ecosystem.MetaActive: "1"}}}})
	if next.(Model).state.ActivePrefixID != "/p" {
		t.Fatal("active prefix not set")
	}
	if cmd == nil {
		t.Fatal("expected load command after discovery resolved the active environment")
	}
}

func TestQuitStillWorks(t *testing.T) {
	m := New(newStubEco())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}
