package app

import (
	"path/filepath"
	"testing"

	"github.com/fon60/alldeps/internal/domain"
	"github.com/fon60/alldeps/internal/ecosystem"
	"github.com/fon60/alldeps/internal/lock"
)

// composerStyleOptions are the four machine-applicable effect kinds the
// composer adapter's classifier produces, in resolver-screen order.
func composerStyleOptions() []ecosystem.ResolutionOption {
	return []ecosystem.ResolutionOption{
		{Label: "install console 7.4.0 (last release supporting php 8.3)", Description: "console latest requires php >=8.4; your platform is php 8.3", Effect: ecosystem.ResolutionEffect{Kind: "install", Name: "console", TargetVersion: "7.4.0"}},
		{Label: "remove http-kernel from the project", Description: "drop the root requirement ^7.0 that console 6.x conflicts with", Effect: ecosystem.ResolutionEffect{Kind: "remove", Name: "http-kernel"}},
		{Label: "upgrade kernel to 6.4", Description: "change the root requirement (currently ^7.0) so both sides match", Effect: ecosystem.ResolutionEffect{Kind: "upgrade", Name: "kernel", TargetVersion: "6.4"}},
		{Label: "Skip installing console", Description: "console will not be applied; the mark is cleared", Effect: ecosystem.ResolutionEffect{Kind: "skip", Name: "console"}},
	}
}

// composerStubModel builds a model whose stub adapter reports one conflict for
// console (marked for install at the unpinned latest 8.1.7) until the intent
// carries any of the resolution effects, mirroring a solver that settles once
// the chosen option is applied.
func composerStubModel(t *testing.T) (Model, *stubEco) {
	t.Helper()
	s := newStubEco()
	s.conflictsFn = func(intent ecosystem.Intent) []ecosystem.Conflict {
		unpinned, settled := false, false
		for _, it := range intent.Items {
			switch {
			case it.Name == "console" && it.Op == ecosystem.OpInstall && it.Version == "8.1.7":
				unpinned = true
			case it.Name == "console" && it.Op == ecosystem.OpInstall && it.Version == "7.4.0":
				settled = true
			case it.Name == "http-kernel" && it.Op == ecosystem.OpRemove:
				settled = true
			case it.Name == "kernel" && it.Op == ecosystem.OpUpgrade && it.Version == "6.4":
				settled = true
			}
		}
		if unpinned && !settled {
			return []ecosystem.Conflict{{Package: "console", Message: "php platform mismatch", Options: composerStyleOptions()}}
		}
		return nil
	}
	m := New(s)
	m.locks = lock.New(filepath.Join(t.TempDir(), "locks"))
	m.state.Prefixes["/p"] = &domain.PrefixState{ID: "/p", Loaded: true, Packages: map[string]*domain.PkgState{
		"console":    {Name: "console", Origin: domain.OriginSearch, LatestVersion: "8.1.7"},
		"http-kernel": {Name: "http-kernel", InstalledVersion: "7.0.3", Origin: domain.OriginInstalled},
		"kernel":     {Name: "kernel", InstalledVersion: "7.0.0", Origin: domain.OriginInstalled},
	}}
	m.state.ActivePrefixID = "/p"
	m.envsByManager["stub"] = []ecosystem.Environment{{ID: "/p"}}
	m.state.Prefixes["/p"].Packages["console"].SetMarkFor("stub", domain.MarkInstall)

	m = m.stepRefresh(t, m.maybeResolveConflicts())
	if len(m.state.Prefixes["/p"].UnresolvedConflicts("console")) != 1 {
		t.Fatal("the initial background probe must report the console conflict")
	}
	return m, s
}

// TestComposerEffectsFlowThroughResolverLoop verifies that choosing each of
// the four composer-style options updates the pending marks per its
// ResolutionEffect and triggers a background re-probe that observes the
// settled state.
func TestComposerEffectsFlowThroughResolverLoop(t *testing.T) {
	tests := []struct {
		name     string
		cursor   int
		reprobe  bool // false when the choice empties the plan (conflicts clear inline, no probe)
		check    func(t *testing.T, m Model)
	}{
		{
			name:    "install pin retargets the mark",
			cursor:  0,
			reprobe: true,
			check: func(t *testing.T, m Model) {
				p := m.state.Prefixes["/p"].Packages["console"]
				if got := p.MarkFor("stub"); got != domain.MarkInstall {
					t.Fatalf("console mark = %v, want MarkInstall", got)
				}
				if got := p.TargetVersionFor("stub"); got != "7.4.0" {
					t.Fatalf("console target = %q, want the pinned 7.4.0", got)
				}
			},
		},
		{
			name:    "remove marks the other root dependency",
			cursor:  1,
			reprobe: true,
			check: func(t *testing.T, m Model) {
				if got := m.state.Prefixes["/p"].Packages["http-kernel"].MarkFor("stub"); got != domain.MarkRemove {
					t.Fatalf("http-kernel mark = %v, want MarkRemove", got)
				}
			},
		},
		{
			name:    "upgrade retargets another installed package",
			cursor:  2,
			reprobe: true,
			check: func(t *testing.T, m Model) {
				p := m.state.Prefixes["/p"].Packages["kernel"]
				if got := p.MarkFor("stub"); got != domain.MarkUpgrade {
					t.Fatalf("kernel mark = %v, want MarkUpgrade", got)
				}
				if got := p.TargetVersionFor("stub"); got != "6.4" {
					t.Fatalf("kernel target = %q, want 6.4", got)
				}
			},
		},
		{
			name:    "skip clears the conflicted mark",
			cursor:  3,
			reprobe: false,
			check: func(t *testing.T, m Model) {
				if got := m.state.Prefixes["/p"].Packages["console"].MarkFor("stub"); got != domain.MarkNone {
					t.Fatalf("console mark = %v, want cleared by the skip option", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := composerStubModel(t)
			m, _ = m.openResolver("/p", "console")
			m.tabs[m.tabIdx].RCursor = tt.cursor

			nextRaw, cmd := m.Update(keyMsg(t, "enter"))
			m = nextRaw.(Model)
			tt.check(t, m)
			if tt.reprobe {
				if cmd == nil {
					t.Fatal("choosing an option must trigger a background re-probe")
				}
				m = m.stepRefresh(t, cmd)
			}
			if got := m.state.Prefixes["/p"].Conflicts; len(got) != 0 {
				t.Fatalf("conflicts after the choice = %+v, want none (the effect settled the clash)", got)
			}
		})
	}
}
