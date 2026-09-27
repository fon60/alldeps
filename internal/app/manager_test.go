package app

import (
	"strings"
	"testing"

	"github.com/fon60/alldeps/internal/domain"
	"github.com/fon60/alldeps/internal/ecosystem"
)

func twoManagerModel(t *testing.T, prefixID string) Model {
	t.Helper()
	m := modelWithLoadedPrefix(t, prefixID, "alpha")
	npm := newStubEco().withID("npm")
	yarn := newStubEco().withID("yarn")
	m.managers = map[string]ecosystem.Ecosystem{"npm": npm, "yarn": yarn}
	m.activeManagerID = "npm"
	m.envsByManager["npm"] = []ecosystem.Environment{{ID: prefixID}}
	m.envsByManager["yarn"] = []ecosystem.Environment{{ID: prefixID}}
	return m
}

func TestExactlyOneManagerActiveAndSwitching(t *testing.T) {
	m := twoManagerModel(t, "/p")
	if m.activeManagerID != "npm" {
		t.Fatalf("initial active manager = %q, want npm", m.activeManagerID)
	}

	m = m.step(t, keyMsg(t, "M"))
	if m.overlay != OverlayManager {
		t.Fatalf("overlay = %v, want the manager switcher", m.overlay)
	}
	out := render80x24(m)
	if strings.Count(out, "*") != 1 {
		t.Fatalf("switcher must mark exactly one manager active:\n%s", out)
	}

	m = m.step(t, keyMsg(t, "j"))
	m = m.step(t, keyMsg(t, "enter"))
	if m.activeManagerID != "yarn" {
		t.Fatalf("active manager after switch = %q, want yarn", m.activeManagerID)
	}
	if m.overlay != OverlayNone {
		t.Fatalf("overlay after select = %v, want none (back to list)", m.overlay)
	}

	m = m.step(t, keyMsg(t, "M"))
	out = render80x24(m)
	if strings.Count(out, "*") != 1 {
		t.Fatalf("after switching, still exactly one manager active:\n%s", out)
	}
}

func TestManagerSwitchPreservesMarksAndInstalledState(t *testing.T) {
	m := twoManagerModel(t, "/p")
	npm := m.managers["npm"].(*stubEco)
	ps := m.state.Prefixes["/p"]
	ps.Packages["alpha"].SetMarkFor("npm", domain.MarkInstall)

	// Switch away to yarn and back; neither switch may re-list.
	m = m.step(t, keyMsg(t, "M"))
	m = m.step(t, keyMsg(t, "j"))
	m = m.step(t, keyMsg(t, "enter"))
	if calls := npm.listCalls; calls != 0 {
		t.Fatalf("switching away re-fetched installed state (%d list calls), want 0", calls)
	}
	m = m.step(t, keyMsg(t, "M"))
	m = m.step(t, keyMsg(t, "k")) // cursor starts on the active manager (yarn); up to npm
	m = m.step(t, keyMsg(t, "enter"))
	if calls := npm.listCalls; calls != 0 {
		t.Fatalf("switching back re-fetched installed state (%d list calls), want 0", calls)
	}

	if got := ps.Packages["alpha"].MarkFor("npm"); got != domain.MarkInstall {
		t.Fatalf("npm mark after round trip = %v, want MarkInstall (marks survive)", got)
	}
	if !ps.Loaded || ps.Packages["alpha"].InstalledVersion != "1.0.0" {
		t.Fatal("installed state changed by the manager switch")
	}
	if m.activeManagerID != "npm" {
		t.Fatalf("active manager = %q, want npm", m.activeManagerID)
	}
}
