package app

import (
	"strings"
	"testing"

	"npmitude/internal/ecosystem"
	"npmitude/internal/domain"
)

func TestSwitchPrefixLoadsTarget(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p/v24", "alpha")
	cmd := m.switchEnv("/p/v22")
	if m.state.ActivePrefixID != "/p/v22" {
		t.Fatalf("active = %q, want /p/v22", m.state.ActivePrefixID)
	}
	if cmd == nil {
		t.Fatal("switching to an unloaded prefix must start a load")
	}

	m2 := modelWithLoadedPrefix(t, "/p/v24", "alpha")
	m2.state.Prefixes["/p/v22"] = &domain.PrefixState{ID: "/p/v22", Packages: map[string]*domain.PkgState{"beta": {Name: "beta", InstalledVersion: "1.0.0"}}, Loaded: true}
	if cmd := m2.switchEnv("/p/v22"); cmd != nil {
		t.Fatal("switching to an already-loaded prefix must not reload")
	}
}

func TestMarksSurviveRoundTrip(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p/v24", "alpha", "beta")
	m.state.SetMark("/p/v24", "alpha", "stub", domain.MarkRemove)
	m.state.SetMark("/p/v24", "beta", "stub", domain.MarkInstall)

	m.switchEnv("/p/v22")
	if m.state.PendingMarkCount("/p/v24", "stub") != 2 {
		t.Fatal("marks lost after switching away")
	}

	m.switchEnv("/p/v24")
	ps := m.state.Prefixes["/p/v24"]
	if ps.Packages["alpha"] .MarkFor("stub") != domain.MarkRemove || ps.Packages["beta"] .MarkFor("stub") != domain.MarkInstall {
		t.Fatal("marks not intact after round trip")
	}
}

func TestCyclePrefix(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p/v24", "alpha")
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p/v24"}, {ID: "/p/v22"}}
	m.switchEnv("/p/v22")
	if m.state.ActivePrefixID != "/p/v22" {
		t.Fatal("first cycle did not move to next prefix")
	}
	m.cycleEnv()
	if m.state.ActivePrefixID != "/p/v24" {
		t.Fatalf("wrap-around failed, active = %q", m.state.ActivePrefixID)
	}
}

func TestVanishedPrefixFallsBackToActive(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p/v22", "beta")
	m.state.ActivePrefixID = "/p/v22"
	m.activeFallback = "/p/v24"
	// v22 vanished from the fresh scan.
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p/v24"}}

	cmd := m.dropVanishedEnvs()
	if m.state.ActivePrefixID != "/p/v24" {
		t.Fatalf("active = %q, want fallback /p/v24", m.state.ActivePrefixID)
	}
	if m.notice == "" || !strings.Contains(m.notice, "no longer exists") {
		t.Fatalf("expected vanished-prefix notice, got %q", m.notice)
	}
	if cmd == nil {
		t.Fatal("fallback prefix must be loaded")
	}
}

func TestVanishedPrefixFallsBackToFirstWhenActiveGone(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p/v22", "beta")
	m.state.ActivePrefixID = "/p/v22"
	m.activeFallback = "/p/gone-active" // active prefix also vanished
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p/v24"}, {ID: "/p/v20"}}

	m.dropVanishedEnvs()
	if m.state.ActivePrefixID != "/p/v24" {
		t.Fatalf("active = %q, want first surviving prefix", m.state.ActivePrefixID)
	}
}

func TestPickerEnterSwitches(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p/v24", "alpha")
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p/v24"}, {ID: "/p/v22"}}
	m.screen = ScreenPicker
	m.pickerCursor = 1

	next, cmd := m.Update(keyMsg(t, "enter"))
	m = next.(Model)
	if m.screen != ScreenList {
		t.Fatal("picker should close on enter")
	}
	if m.state.ActivePrefixID != "/p/v22" {
		t.Fatalf("active = %q, want /p/v22", m.state.ActivePrefixID)
	}
	if cmd == nil {
		t.Fatal("expected load command for the newly selected prefix")
	}
}

func TestPickerEscReturns(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p/v24", "alpha")
	m.screen = ScreenPicker
	next, _ := m.Update(keyMsg(t, "esc"))
	if next.(Model).screen != ScreenList {
		t.Fatal("esc should return to the list")
	}
}

func TestRefreshReloadsAndDropsVanished(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p/v24", "alpha")
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p/v24"}}

	// A re-scan in which everything (including the active environment)
	// vanished must surface a notice.
	next, _ := m.Update(discoverMsg{envsByManager: map[string][]ecosystem.Environment{"stub": {}}})
	m = next.(Model)
	if m.notice == "" {
		t.Fatal("expected a notice after failed refresh")
	}
}
