package app

import (
	"testing"

	"github.com/fon60/alldeps/internal/domain"
)

func TestPlusInstallsNotInstalled(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p")
	m.state.Prefixes["/p"].Packages["ghost"] = &domain.PkgState{Name: "ghost", Origin: domain.OriginSearch, LatestVersion: "1.0.0"}
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["ghost"].MarkFor("stub"); got != domain.MarkInstall {
		t.Fatalf("mark = %v, want MarkInstall", got)
	}
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["ghost"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("mark after toggle = %v, want MarkNone", got)
	}
}

func TestPlusUpgradesInstalledOutdated(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.Prefixes["/p"].Packages["alpha"].LatestVersion = "2.0.0"
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].MarkFor("stub"); got != domain.MarkUpgrade {
		t.Fatalf("mark = %v, want MarkUpgrade", got)
	}
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("mark after toggle = %v, want MarkNone", got)
	}
}

func TestPlusOnCurrentInstalledIsNoOp(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.Prefixes["/p"].Packages["alpha"].LatestVersion = "1.0.0" // == installed
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("mark = %v, want MarkNone", got)
	}
	if m.notice == "" {
		t.Fatal("expected a notice explaining nothing was marked")
	}
}

func TestPlusOnInstalledUnknownLatestIsNoOp(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "+"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("mark = %v, want MarkNone", got)
	}
	if m.notice == "" {
		t.Fatal("expected a notice about unknown latest version")
	}
}

func TestRemoveHoldRevert(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "-"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].MarkFor("stub"); got != domain.MarkRemove {
		t.Fatalf("mark = %v, want MarkRemove", got)
	}
	m = m.step(t, keyMsg(t, "="))
	if got := m.state.Prefixes["/p"].Packages["alpha"].MarkFor("stub"); got != domain.MarkHold {
		t.Fatalf("mark = %v, want MarkHold", got)
	}
	m = m.step(t, keyMsg(t, ":"))
	if got := m.state.Prefixes["/p"].Packages["alpha"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("mark after revert = %v, want MarkNone", got)
	}
}

func TestRemoveIgnoresNotInstalled(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p")
	m.state.Prefixes["/p"].Packages["ghost"] = &domain.PkgState{Name: "ghost", Origin: domain.OriginSearch, LatestVersion: "1.0.0"}
	m = m.step(t, keyMsg(t, "-"))
	if got := m.state.Prefixes["/p"].Packages["ghost"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("remove on not-installed set mark %v, want none", got)
	}
}

func TestMarkAllUpgradableRespectsHolds(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "a", "b", "c", "d", "e")
	ps := m.state.Prefixes["/p"]
	for _, n := range []string{"a", "b", "c"} {
		ps.Packages[n].LatestVersion = "9.9.9" // outdated
	}
	m.state.SetMark("/p", "d", "stub", domain.MarkHold)

	m = m.step(t, keyMsg(t, "U"))

	for _, n := range []string{"a", "b", "c"} {
		if got := ps.Packages[n].MarkFor("stub"); got != domain.MarkUpgrade {
			t.Fatalf("%s mark = %v, want MarkUpgrade", n, got)
		}
	}
	if got := ps.Packages["d"].MarkFor("stub"); got != domain.MarkHold {
		t.Fatalf("held d mark = %v, want MarkHold untouched", got)
	}
	if got := ps.Packages["e"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("current e mark = %v, want MarkNone", got)
	}
}

func TestClearAllMarks(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	m.state.SetMark("/p", "alpha", "stub", domain.MarkInstall)
	m.state.SetMark("/p", "beta", "stub", domain.MarkRemove)

	m = m.step(t, keyMsg(t, "x"))

	if got := m.state.PendingMarkCount("/p", "stub"); got != 0 {
		t.Fatalf("pending after clear = %d, want 0", got)
	}
}

func TestMarksScopedPerPrefix(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p1", "alpha")
	m.state.Prefixes["/p1"].Packages["alpha"].LatestVersion = "2.0.0" // upgradable
	m.state.Prefixes["/p2"] = &domain.PrefixState{ID: "/p2", Packages: map[string]*domain.PkgState{
		"alpha": {Name: "alpha", InstalledVersion: "1.0.0", Origin: domain.OriginInstalled},
	}, Loaded: true}
	m = m.step(t, keyMsg(t, "+"))

	if got := m.state.Prefixes["/p1"].Packages["alpha"].MarkFor("stub"); got != domain.MarkUpgrade {
		t.Fatalf("p1 mark = %v, want MarkUpgrade", got)
	}
	if got := m.state.Prefixes["/p2"].Packages["alpha"].MarkFor("stub"); got != domain.MarkNone {
		t.Fatalf("p2 mark = %v, want MarkNone (marks are per-prefix)", got)
	}
}
