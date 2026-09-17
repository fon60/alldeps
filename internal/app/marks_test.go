package app

import (
	"testing"

	"npmitude/internal/state"
)

func TestPlusInstallsNotInstalled(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p")
	m.state.Prefixes["/p"].Packages["ghost"] = &state.PkgState{Name: "ghost", Origin: state.OriginSearch, LatestVersion: "1.0.0"}
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["ghost"].Mark; got != state.MarkInstall {
		t.Fatalf("mark = %v, want MarkInstall", got)
	}
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["ghost"].Mark; got != state.MarkNone {
		t.Fatalf("mark after toggle = %v, want MarkNone", got)
	}
}

func TestPlusUpgradesInstalledOutdated(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.Prefixes["/p"].Packages["alpha"].LatestVersion = "2.0.0"
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].Mark; got != state.MarkUpgrade {
		t.Fatalf("mark = %v, want MarkUpgrade", got)
	}
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].Mark; got != state.MarkNone {
		t.Fatalf("mark after toggle = %v, want MarkNone", got)
	}
}

func TestPlusOnCurrentInstalledIsNoOp(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.Prefixes["/p"].Packages["alpha"].LatestVersion = "1.0.0" // == installed
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].Mark; got != state.MarkNone {
		t.Fatalf("mark = %v, want MarkNone", got)
	}
	if m.notice == "" {
		t.Fatal("expected a notice explaining nothing was marked")
	}
}

func TestPlusOnInstalledUnknownLatestIsNoOp(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].Mark; got != state.MarkNone {
		t.Fatalf("mark = %v, want MarkNone", got)
	}
	if m.notice == "" {
		t.Fatal("expected a notice about unknown latest version")
	}
}

func TestRemoveHoldRevert(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "-"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].Mark; got != state.MarkRemove {
		t.Fatalf("mark = %v, want MarkRemove", got)
	}
	m = m.step(t, keyMsg(t, "="))
	if got := m.state.Prefixes["/p"].Packages["alpha"].Mark; got != state.MarkHold {
		t.Fatalf("mark = %v, want MarkHold", got)
	}
	m = m.step(t, keyMsg(t, ":"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].Mark; got != state.MarkNone {
		t.Fatalf("mark after revert = %v, want MarkNone", got)
	}
}

func TestRemoveIgnoresNotInstalled(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p")
	m.state.Prefixes["/p"].Packages["ghost"] = &state.PkgState{Name: "ghost", Origin: state.OriginSearch, LatestVersion: "1.0.0"}
	m = m.step(t, keyMsg(t, "-"))
	if got := m.state.Prefixes["/p"].Packages["ghost"].Mark; got != state.MarkNone {
		t.Fatalf("remove on not-installed set mark %v, want none", got)
	}
}

func TestMarkAllUpgradableRespectsHolds(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "a", "b", "c", "d", "e")
	ps := m.state.Prefixes["/p"]
	for _, n := range []string{"a", "b", "c"} {
		ps.Packages[n].LatestVersion = "9.9.9" // outdated
	}
	m.state.SetMark("/p", "d", state.MarkHold)

	m = m.step(t, keyMsg(t, "U"))

	for _, n := range []string{"a", "b", "c"} {
		if got := ps.Packages[n].Mark; got != state.MarkUpgrade {
			t.Fatalf("%s mark = %v, want MarkUpgrade", n, got)
		}
	}
	if got := ps.Packages["d"].Mark; got != state.MarkHold {
		t.Fatalf("held d mark = %v, want MarkHold untouched", got)
	}
	if got := ps.Packages["e"].Mark; got != state.MarkNone {
		t.Fatalf("current e mark = %v, want MarkNone", got)
	}
}

func TestClearAllMarks(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	m.state.SetMark("/p", "alpha", state.MarkInstall)
	m.state.SetMark("/p", "beta", state.MarkRemove)

	m = m.step(t, keyMsg(t, "x"))

	if got := m.state.PendingMarkCount("/p"); got != 0 {
		t.Fatalf("pending after clear = %d, want 0", got)
	}
}

func TestMarksScopedPerPrefix(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p1", "alpha")
	m.state.Prefixes["/p1"].Packages["alpha"].LatestVersion = "2.0.0" // upgradable
	m.state.Prefixes["/p2"] = &state.PrefixState{ID: "/p2", Packages: map[string]*state.PkgState{
		"alpha": {Name: "alpha", InstalledVersion: "1.0.0", Origin: state.OriginInstalled},
	}, Loaded: true}
	m = m.step(t, keyMsg(t, "+"))

	if got := m.state.Prefixes["/p1"].Packages["alpha"].Mark; got != state.MarkUpgrade {
		t.Fatalf("p1 mark = %v, want MarkUpgrade", got)
	}
	if got := m.state.Prefixes["/p2"].Packages["alpha"].Mark; got != state.MarkNone {
		t.Fatalf("p2 mark = %v, want MarkNone (marks are per-prefix)", got)
	}
}
