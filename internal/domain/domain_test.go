package domain

import "testing"

func twoPrefixFixture() *AppState {
	s := NewAppState()
	mk := func(prefix, name, ver string) *PkgState {
		p := &PkgState{Name: name, InstalledVersion: ver, Origin: OriginInstalled}
		s.Prefixes[prefix] = ensurePrefix(s, prefix)
		s.Prefixes[prefix].Packages[name] = p
		return p
	}
	mk("/p/v24", "alpha", "1.0.0")
	mk("/p/v24", "beta", "2.0.0")
	mk("/p/v22", "alpha", "0.9.0")
	s.ActivePrefixID = "/p/v24"
	return s
}

func ensurePrefix(s *AppState, id string) *PrefixState {
	ps := s.Prefixes[id]
	if ps == nil {
		ps = &PrefixState{ID: id, Packages: map[string]*PkgState{}}
		s.Prefixes[id] = ps
	}
	return ps
}

func TestSetMark(t *testing.T) {
	s := twoPrefixFixture()
	s.SetMark("/p/v24", "alpha", "npm", MarkRemove)
	if got := s.Prefixes["/p/v24"].Packages["alpha"].FlagFor("npm"); got != "i-" {
		t.Fatalf("flag = %q, want i-", got)
	}
	if n := s.PendingMarkCount("/p/v24", "npm"); n != 1 {
		t.Fatalf("pending = %d, want 1", n)
	}
}

func TestSetMarkToggleOff(t *testing.T) {
	s := twoPrefixFixture()
	s.SetMark("/p/v24", "alpha", "npm", MarkRemove)
	s.SetMark("/p/v24", "alpha", "npm", MarkRemove)
	if got := s.Prefixes["/p/v24"].Packages["alpha"].FlagFor("npm"); got != "i*" {
		t.Fatalf("flag = %q, want i* after toggle-off", got)
	}
	if n := s.PendingMarkCount("/p/v24", "npm"); n != 0 {
		t.Fatalf("pending = %d, want 0", n)
	}
}

func TestSetMarkReplaceDifferent(t *testing.T) {
	s := twoPrefixFixture()
	s.SetMark("/p/v24", "alpha", "npm", MarkRemove)
	s.SetMark("/p/v24", "alpha", "npm", MarkHold)
	if got := s.Prefixes["/p/v24"].Packages["alpha"].FlagFor("npm"); got != "ih" {
		t.Fatalf("flag = %q, want ih", got)
	}
}

func TestRevert(t *testing.T) {
	s := twoPrefixFixture()
	s.SetMark("/p/v24", "beta", "npm", MarkInstall)
	s.Revert("/p/v24", "beta", "npm")
	if got := s.Prefixes["/p/v24"].Packages["beta"].FlagFor("npm"); got != "i*" {
		t.Fatalf("flag = %q, want i*", got)
	}
}

func TestClearAllMarks(t *testing.T) {
	s := twoPrefixFixture()
	s.SetMark("/p/v24", "alpha", "npm", MarkRemove)
	s.SetMark("/p/v24", "beta", "npm", MarkInstall)
	s.ClearAllMarks("/p/v24", "npm")
	if n := s.PendingMarkCount("/p/v24", "npm"); n != 0 {
		t.Fatalf("pending = %d, want 0", n)
	}
}

func TestPerPrefixMarkIsolation(t *testing.T) {
	s := twoPrefixFixture()
	// Mark on v24 must not touch the same-named package on v22.
	s.SetMark("/p/v24", "alpha", "npm", MarkRemove)
	if got := s.Prefixes["/p/v22"].Packages["alpha"].FlagFor("npm"); got != "i*" {
		t.Fatalf("v22 alpha flag = %q, want i* (unaffected)", got)
	}
	if n := s.PendingMarkCount("/p/v22", "npm"); n != 0 {
		t.Fatalf("v22 pending = %d, want 0", n)
	}

	// Clearing marks on v22 must not clear v24's mark.
	s.SetMark("/p/v22", "alpha", "npm", MarkHold)
	s.ClearAllMarks("/p/v22", "npm")
	if got := s.Prefixes["/p/v24"].Packages["alpha"].FlagFor("npm"); got != "i-" {
		t.Fatalf("v24 alpha flag = %q, want i- (preserved)", got)
	}
}

func TestMarkAllUpgradableRespectsHolds(t *testing.T) {
	s := twoPrefixFixture()
	a := s.Prefixes["/p/v24"].Packages["alpha"]
	b := s.Prefixes["/p/v24"].Packages["beta"]
	a.LatestVersion = "1.1.0" // upgradable
	b.LatestVersion = "3.0.0" // upgradable but held
	s.SetMark("/p/v24", "beta", "npm", MarkHold)

	n := s.MarkAllUpgradable("/p/v24", "npm")
	if n != 1 {
		t.Fatalf("marked %d, want 1 (held package excluded)", n)
	}
	if got := a.FlagFor("npm"); got != "iu" {
		t.Fatalf("alpha flag = %q, want iu", got)
	}
	if got := b.FlagFor("npm"); got != "ih" {
		t.Fatalf("beta flag = %q, want ih (hold kept)", got)
	}
}

func TestGenericHealthFlag(t *testing.T) {
	p := &PkgState{Name: "x", InstalledVersion: "1.0.0"}
	if p.StateChar() != 'i' {
		t.Fatalf("healthy installed row state char = %c, want i", p.StateChar())
	}
	p.Unhealthy = true
	if p.StateChar() != 'b' {
		t.Fatalf("unhealthy row state char = %c, want b (takes precedence over installed)", p.StateChar())
	}
	if p.FlagFor("npm") != "b*" {
		t.Fatalf("unhealthy flag = %q, want b*", p.FlagFor("npm"))
	}
}

func TestFlagChars(t *testing.T) {
	cases := []struct {
		pkg  PkgState
		want string
	}{
		{PkgState{Name: "x", InstalledVersion: "1.0.0"}, "i*"},
		{PkgState{Name: "x"}, "p*"},
		{PkgState{Name: "x", Unhealthy: true, InstalledVersion: "1.0.0"}, "b*"},
		{PkgState{Name: "x", Marks: map[string]MarkEntry{"npm": {Mark: MarkInstall}}}, "p+"},
	}
	for i, c := range cases {
		if got := c.pkg.FlagFor("npm"); got != c.want {
			t.Fatalf("case %d: flag = %q, want %q", i, got, c.want)
		}
	}
}
