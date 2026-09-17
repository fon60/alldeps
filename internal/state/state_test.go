package state

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
	s.SetMark("/p/v24", "alpha", MarkRemove)
	if got := s.Prefixes["/p/v24"].Packages["alpha"].Flag(); got != "i-" {
		t.Fatalf("flag = %q, want i-", got)
	}
	if n := s.PendingMarkCount("/p/v24"); n != 1 {
		t.Fatalf("pending = %d, want 1", n)
	}
}

func TestSetMarkToggleOff(t *testing.T) {
	s := twoPrefixFixture()
	s.SetMark("/p/v24", "alpha", MarkRemove)
	s.SetMark("/p/v24", "alpha", MarkRemove)
	if got := s.Prefixes["/p/v24"].Packages["alpha"].Flag(); got != "i*" {
		t.Fatalf("flag = %q, want i* after toggle-off", got)
	}
	if n := s.PendingMarkCount("/p/v24"); n != 0 {
		t.Fatalf("pending = %d, want 0", n)
	}
}

func TestSetMarkReplaceDifferent(t *testing.T) {
	s := twoPrefixFixture()
	s.SetMark("/p/v24", "alpha", MarkRemove)
	s.SetMark("/p/v24", "alpha", MarkHold)
	if got := s.Prefixes["/p/v24"].Packages["alpha"].Flag(); got != "ih" {
		t.Fatalf("flag = %q, want ih", got)
	}
}

func TestRevert(t *testing.T) {
	s := twoPrefixFixture()
	s.SetMark("/p/v24", "beta", MarkInstall)
	s.Revert("/p/v24", "beta")
	if got := s.Prefixes["/p/v24"].Packages["beta"].Flag(); got != "i*" {
		t.Fatalf("flag = %q, want i*", got)
	}
}

func TestClearAllMarks(t *testing.T) {
	s := twoPrefixFixture()
	s.SetMark("/p/v24", "alpha", MarkRemove)
	s.SetMark("/p/v24", "beta", MarkInstall)
	s.ClearAllMarks("/p/v24")
	if n := s.PendingMarkCount("/p/v24"); n != 0 {
		t.Fatalf("pending = %d, want 0", n)
	}
}

func TestPerPrefixMarkIsolation(t *testing.T) {
	s := twoPrefixFixture()
	// Mark on v24 must not touch the same-named package on v22.
	s.SetMark("/p/v24", "alpha", MarkRemove)
	if got := s.Prefixes["/p/v22"].Packages["alpha"].Flag(); got != "i*" {
		t.Fatalf("v22 alpha flag = %q, want i* (unaffected)", got)
	}
	if n := s.PendingMarkCount("/p/v22"); n != 0 {
		t.Fatalf("v22 pending = %d, want 0", n)
	}

	// Clearing marks on v22 must not clear v24's mark.
	s.SetMark("/p/v22", "alpha", MarkHold)
	s.ClearAllMarks("/p/v22")
	if got := s.Prefixes["/p/v24"].Packages["alpha"].Flag(); got != "i-" {
		t.Fatalf("v24 alpha flag = %q, want i- (preserved)", got)
	}
}

func TestMarkAllUpgradableRespectsHolds(t *testing.T) {
	s := twoPrefixFixture()
	a := s.Prefixes["/p/v24"].Packages["alpha"]
	b := s.Prefixes["/p/v24"].Packages["beta"]
	a.LatestVersion = "1.1.0" // upgradable
	b.LatestVersion = "3.0.0" // upgradable but held
	s.SetMark("/p/v24", "beta", MarkHold)

	n := s.MarkAllUpgradable("/p/v24")
	if n != 1 {
		t.Fatalf("marked %d, want 1 (held package excluded)", n)
	}
	if got := a.Flag(); got != "iu" {
		t.Fatalf("alpha flag = %q, want iu", got)
	}
	if got := b.Flag(); got != "ih" {
		t.Fatalf("beta flag = %q, want ih (hold kept)", got)
	}
}

func TestFlagChars(t *testing.T) {
	cases := []struct {
		pkg  PkgState
		want string
	}{
		{PkgState{Name: "x", InstalledVersion: "1.0.0"}, "i*"},
		{PkgState{Name: "x"}, "p*"},
		{PkgState{Name: "x", Broken: true, InstalledVersion: "1.0.0"}, "b*"},
		{PkgState{Name: "x", Mark: MarkInstall}, "p+"},
	}
	for i, c := range cases {
		if got := c.pkg.Flag(); got != c.want {
			t.Fatalf("case %d: flag = %q, want %q", i, got, c.want)
		}
	}
}
