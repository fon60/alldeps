package domain

import "testing"

func unifiedFixture() []*PrefixState {
	mk := func(id, name, ver string) *PrefixState {
		ps := &PrefixState{ID: id, Packages: map[string]*PkgState{}, Loaded: true}
		if ver != "" {
			ps.Packages[name] = &PkgState{Name: name, InstalledVersion: ver, Origin: OriginInstalled}
		} else {
			ps.Packages[name] = &PkgState{Name: name, Origin: OriginSearch, LatestVersion: "9.0.0"}
		}
		return ps
	}
	v24 := mk("/p/v24", "alpha", "1.0.0")
	v24.Packages["beta"] = &PkgState{Name: "beta", InstalledVersion: "2.0.0", Origin: OriginInstalled}
	v18 := mk("/p/v18", "alpha", "0.9.0")
	v16 := mk("/p/v16", "alpha", "0.8.0")
	return []*PrefixState{v24, v18, v16}
}

func TestGroupByNameMultiDestination(t *testing.T) {
	ranks := map[string]string{"/p/v24": "v24.0.0", "/p/v18": "v18.0.0", "/p/v16": "v16.0.0"}
	rows := GroupByName(unifiedFixture(), ranks)

	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (one per name)", len(rows))
	}
	alpha := rows[0]
	if alpha.Name != "alpha" {
		t.Fatalf("first row = %q, want alpha", alpha.Name)
	}
	if !alpha.Installed() || alpha.Headline.InstalledVersion != "1.0.0" {
		t.Fatalf("alpha headline = %+v, want the v24 copy 1.0.0", alpha.Headline)
	}
	if alpha.Count != 2 {
		t.Fatalf("alpha count = %d, want 2 additional destinations", alpha.Count)
	}
	beta := rows[1]
	if beta.Name != "beta" || beta.Count != 0 {
		t.Fatalf("beta row wrong: %+v (single destination must have count 0)", beta)
	}
}

func TestGroupByNameSingleDestination(t *testing.T) {
	ps := &PrefixState{ID: "/p/v24", Packages: map[string]*PkgState{
		"alpha": {Name: "alpha", InstalledVersion: "1.0.0", Origin: OriginInstalled},
	}, Loaded: true}
	rows := GroupByName([]*PrefixState{ps}, map[string]string{"/p/v24": "v24.0.0"})
	if len(rows) != 1 || rows[0].Headline == nil || rows[0].Count != 0 {
		t.Fatalf("rows = %+v, want one installed row with count 0", rows)
	}
}

func TestGroupByNameEmpty(t *testing.T) {
	if rows := GroupByName(nil, nil); len(rows) != 0 {
		t.Fatalf("rows = %v, want none", rows)
	}
	ps := &PrefixState{ID: "/p/v24", Packages: map[string]*PkgState{}, Loaded: true}
	if rows := GroupByName([]*PrefixState{ps}, nil); len(rows) != 0 {
		t.Fatalf("rows = %v, want none for an empty prefix", rows)
	}
}

func TestGroupByNameSearchOnlyRow(t *testing.T) {
	ps := &PrefixState{ID: "/p/v24", Packages: map[string]*PkgState{
		"ghost": {Name: "ghost", Origin: OriginSearch, LatestVersion: "3.0.0"},
	}, Loaded: true}
	rows := GroupByName([]*PrefixState{ps}, nil)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	u := rows[0]
	if u.Installed() || u.Headline != nil {
		t.Fatalf("search-only row must not be installed: %+v", u)
	}
	if u.Row == nil || u.Row.LatestVersion != "3.0.0" {
		t.Fatalf("representative row = %+v, want the search row", u.Row)
	}
	if u.Flag("npm") != "p*" {
		t.Fatalf("flag = %q, want p*", u.Flag("npm"))
	}
}

func TestGroupByNameMissingRankFallsBackToID(t *testing.T) {
	a := &PrefixState{ID: "/a", Packages: map[string]*PkgState{"x": {Name: "x", InstalledVersion: "1.0.0"}}, Loaded: true}
	b := &PrefixState{ID: "/b", Packages: map[string]*PkgState{"x": {Name: "x", InstalledVersion: "2.0.0"}}, Loaded: true}
	// No ranks at all: the lower id must headline deterministically.
	rows := GroupByName([]*PrefixState{b, a}, nil)
	if rows[0].Headline.InstalledVersion != "1.0.0" {
		t.Fatalf("headline = %q, want /a's copy (id tie-break)", rows[0].Headline.InstalledVersion)
	}
}

func TestGroupByNameFlagAggregatesMarks(t *testing.T) {
	fx := unifiedFixture()
	for _, ps := range fx {
		if p := ps.Packages["alpha"]; p != nil {
			p.SetMarkFor("npm", MarkRemove)
		}
	}
	rows := GroupByName(fx, map[string]string{"/p/v24": "v24.0.0"})
	if rows[0].Flag("npm") != "i-" {
		t.Fatalf("flag = %q, want i- (remove marked on a destination)", rows[0].Flag("npm"))
	}
	// A mark under another manager must not leak into npm's flag.
	fx[0].Packages["alpha"].SetMarkFor("yarn", MarkInstall)
	if got := GroupByName(fx, map[string]string{"/p/v24": "v24.0.0"})[0].Flag("npm"); got != "i-" {
		t.Fatalf("flag = %q, want i- (yarn mark must not affect npm)", got)
	}
	if got := GroupByName(fx, map[string]string{"/p/v24": "v24.0.0"})[0].Flag("yarn"); got != "i+" {
		t.Fatalf("yarn flag = %q, want i+", got)
	}
}
