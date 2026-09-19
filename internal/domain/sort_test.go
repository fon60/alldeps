package domain

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.2.0", "1.10.0", -1},
		{"2.0.0", "10.0.0", -1},
		{"1.0", "1.0.0", 0},
		{"1.0.1", "1.0", 1},
		{"v1.2.3", "1.2.3", 0},
		{"1.0.0-beta", "1.0.0", -1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.2", -1},
		{"1.0.0-2", "1.0.0-11", -1},
		{"1.0.0-1", "1.0.0-alpha", -1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func mkRows(vers map[string]string, sizes map[string]int64) []*PkgState {
	rows := make([]*PkgState, 0, len(vers))
	for name, v := range vers {
		r := &PkgState{Name: name, InstalledVersion: v}
		if s, ok := sizes[name]; ok {
			b := s
			r.SizeBytes = &b
		}
		rows = append(rows, r)
	}
	return rows
}

func sortedNames(rows []*PkgState) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

func TestSortByVersion(t *testing.T) {
	rows := mkRows(map[string]string{"zeta": "2.0.0", "alpha": "10.0.0", "mid": "1.9.0"}, nil)
	SortRows(rows, SortVersion)
	if got := sortedNames(rows); len(got) != 3 || got[0] != "mid" || got[1] != "zeta" || got[2] != "alpha" {
		t.Fatalf("version sort = %v", got)
	}
}

func TestSortBySize(t *testing.T) {
	sizes := map[string]int64{"big": 300, "small": 100}
	rows := mkRows(map[string]string{"big": "1.0.0", "small": "1.0.0", "unknown": "1.0.0"}, sizes)
	SortRows(rows, SortSize)
	if got := sortedNames(rows); got[0] != "small" || got[1] != "big" || got[2] != "unknown" {
		t.Fatalf("size sort = %v (unknown must be last)", got)
	}
}

func TestSortByState(t *testing.T) {
	rows := []*PkgState{
		{Name: "p1", InstalledVersion: "1.0.0"},               // i*
		{Name: "b1", InstalledVersion: "1.0.0", Unhealthy: true}, // b*
		{Name: "s1"}, // p*
		{Name: "i1", InstalledVersion: "1.0.0", Mark: MarkInstall}, // i+
	}
	SortRows(rows, SortState)
	got := sortedNames(rows)
	want := []string{"b1", "p1", "i1", "s1"} // b* < i* < i+ < p* (ASCII: '*' < '+')
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("state sort = %v, want %v", got, want)
		}
	}
}

func TestSortByNameDefault(t *testing.T) {
	rows := mkRows(map[string]string{"zeta": "1.0.0", "alpha": "2.0.0"}, nil)
	SortRows(rows, SortName)
	if got := sortedNames(rows); got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("name sort = %v", got)
	}
}

func TestSortKeyCycle(t *testing.T) {
	k := SortName
	if k.Next() != SortVersion || k.Next().Next() != SortSize || k.Next().Next().Next() != SortState || k.Next().Next().Next().Next() != SortName {
		t.Fatal("sort key cycle broken")
	}
}
