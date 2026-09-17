package app

import (
	"path/filepath"
	"strings"
	"testing"

	"npmitude/internal/lock"
	"npmitude/internal/state"
)

// foreignHolder acquires a live lock (this test process is alive, so its
// PID + start time pass the liveness check) in the same lock dir the app
// would use.
func foreignHolder(t *testing.T, stateDir, prefixID string) {
	t.Helper()
	other := lock.New(filepath.Join(stateDir, "npmitude", "locks"))
	if err := other.Acquire(prefixID); err != nil {
		t.Fatal(err)
	}
}

func TestStartupRefusesLockedDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	m := New()
	foreignHolder(t, dir, "/p")

	nextRaw, cmd := m.Update(activePrefixMsg{prefixID: "/p"})
	next := nextRaw.(Model)
	if cmd != nil {
		t.Fatal("no load command should start for a locked default environment")
	}
	if next.screen != ScreenPicker {
		t.Fatalf("screen = %v, want picker", next.screen)
	}
	if !strings.Contains(next.notice, "another npmitude") {
		t.Fatalf("notice = %q, want holder identification", next.notice)
	}
	if next.state.ActivePrefixID != "" {
		t.Fatal("active prefix must stay unset while the default is locked")
	}
}

func TestSwitchRefusesLockedEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	m := New()
	foreignHolder(t, dir, "/a")
	if err := m.locks.Acquire("/b"); err != nil {
		t.Fatal(err)
	}
	m.state.ActivePrefixID = "/b"
	m.state.Prefixes["/b"] = &state.PrefixState{ID: "/b", Packages: map[string]*state.PkgState{}, Loaded: true}

	if cmd := m.switchPrefix("/a"); cmd != nil {
		t.Fatal("switch to a locked environment must not load")
	}
	if m.state.ActivePrefixID != "/b" {
		t.Fatalf("active = %q, want unchanged /b", m.state.ActivePrefixID)
	}
	if !strings.Contains(m.notice, "another npmitude") {
		t.Fatalf("notice = %q, want holder identification", m.notice)
	}
}

func TestQuitReleasesLocks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	m := New()
	if err := m.locks.Acquire("/b"); err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(keyMsg(t, "q"))
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if m.locks.IsHeld("/b") {
		t.Fatal("quit must release all held environment locks")
	}
}
