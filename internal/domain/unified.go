package domain

import "sort"

// UnifiedRow is one package name aggregated across all destinations of a
// manager. Headline is the installed copy in the highest-ranked destination
// that has it (nil when no destination has it installed); Count is how many
// additional destinations also have it installed; Row is the representative
// row to render (the headline, else the search-origin row).
type UnifiedRow struct {
	Name       string
	Headline   *PkgState
	HeadlineID string // destination of the headline copy ("" when not installed)
	Count      int
	Row        *PkgState

	copies []*PkgState // all installed copies, headline first
}

// Installed reports whether any destination carries the package.
func (u *UnifiedRow) Installed() bool { return u.Headline != nil }

// Unhealthy reports whether any installed copy is unhealthy.
func (u *UnifiedRow) Unhealthy() bool {
	for _, p := range u.copies {
		if p.Unhealthy {
			return true
		}
	}
	return false
}

// Upgradable reports whether any installed copy has a newer version known.
func (u *UnifiedRow) Upgradable() bool {
	for _, p := range u.copies {
		if p.Upgradable() {
			return true
		}
	}
	return false
}

// StateChar is the aggregated state flag character: b when any copy is
// unhealthy, i when installed anywhere, p otherwise.
func (u *UnifiedRow) StateChar() rune {
	if u.Unhealthy() {
		return 'b'
	}
	if u.Installed() {
		return 'i'
	}
	return 'p'
}

// ActionSummary is the aggregated action flag character across destinations:
// install outranks remove, remove upgrade, upgrade hold.
func (u *UnifiedRow) ActionSummary(manager string) rune {
	best := MarkNone
	for _, p := range u.copies {
		if mk := p.MarkFor(manager); rankMark(mk) > rankMark(best) {
			best = mk
		}
	}
	return actionChar(best)
}

// Flag is the two-character state/action flag of the aggregated row for one
// manager (e.g. "i+", "p*").
func (u *UnifiedRow) Flag(manager string) string {
	return string(u.StateChar()) + string(u.ActionSummary(manager))
}

// SortVersion is the version used when sorting unified rows: the headline's
// installed version, else the representative row's known latest.
func (u *UnifiedRow) SortVersion() string {
	if u.Headline != nil {
		return u.Headline.InstalledVersion
	}
	if u.Row != nil {
		return u.Row.LatestVersion
	}
	return ""
}

// SizeBytes is the headline's measured size, else the representative row's.
func (u *UnifiedRow) SizeBytes() *int64 {
	if u.Headline != nil {
		return u.Headline.SizeBytes
	}
	if u.Row != nil {
		return u.Row.SizeBytes
	}
	return nil
}

type agg struct {
	headline *PkgState
	headRank string
	headID   string
	copies   []*PkgState // installed copies, headline first (rebuilt at the end)
	search   *PkgState
}

// GroupByName aggregates the packages of the given destinations by name.
// ranks maps destination id to its comparable rank (Node/runtime version);
// missing or tied ranks fall back to a stable destination-id order. The result
// is sorted by package name.
func GroupByName(prefixes []*PrefixState, ranks map[string]string) []UnifiedRow {
	order := make([]*PrefixState, 0, len(prefixes))
	for _, ps := range prefixes {
		if ps != nil {
			order = append(order, ps)
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		return rankBefore(ranks[order[i].ID], order[i].ID, ranks[order[j].ID], order[j].ID)
	})

	byName := map[string]*agg{}
	var names []string
	for _, ps := range order {
		for _, p := range ps.Rows() {
			a, ok := byName[p.Name]
			if !ok {
				a = &agg{}
				byName[p.Name] = a
				names = append(names, p.Name)
			}
			if p.Installed() {
				r := ranks[ps.ID]
				if a.headline == nil || rankBefore(r, ps.ID, a.headRank, a.headID) {
					a.copies = append([]*PkgState{p}, a.copies...)
					a.headline, a.headRank, a.headID = p, r, ps.ID
				} else {
					a.copies = append(a.copies, p)
				}
			} else if a.search == nil && p.Origin == OriginSearch {
				a.search = p
			}
		}
	}

	out := make([]UnifiedRow, 0, len(names))
	for _, name := range names {
		a := byName[name]
		u := UnifiedRow{Name: name, copies: a.copies}
		if a.headline != nil {
			u.Headline = a.headline
			u.HeadlineID = a.headID
			u.Count = len(a.copies) - 1
			u.Row = a.headline
		} else if a.search != nil {
			u.Row = a.search
		}
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// rankBefore reports whether destination (r1,id1) ranks before (r2,id2):
// higher version first, missing ranks last, id as the stable tie-breaker.
func rankBefore(r1, id1, r2, id2 string) bool {
	switch {
	case r1 == "" && r2 == "":
		return id1 < id2
	case r1 == "":
		return false
	case r2 == "":
		return true
	default:
		if c := CompareVersions(r1, r2); c != 0 {
			return c > 0
		}
		return id1 < id2
	}
}

func rankMark(mk Mark) int {
	switch mk {
	case MarkInstall:
		return 4
	case MarkRemove:
		return 3
	case MarkUpgrade:
		return 2
	case MarkHold:
		return 1
	default:
		return 0
	}
}

// SortUnified sorts unified rows in place by key, mirroring SortRows
// semantics; the name order is the tie-breaker everywhere.
func SortUnified(rows []UnifiedRow, key SortKey) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := &rows[i], &rows[j]
		switch key {
		case SortVersion:
			if c := CompareVersions(a.SortVersion(), b.SortVersion()); c != 0 {
				return c < 0
			}
		case SortSize:
			as, bs := a.SizeBytes(), b.SizeBytes()
			switch {
			case as == nil && bs == nil:
			case as == nil:
				return false
			case bs == nil:
				return true
			default:
				if *as != *bs {
					return *as < *bs
				}
			}
		case SortState:
			if fa, fb := a.StateChar(), b.StateChar(); fa != fb {
				return fa < fb
			}
		}
		return a.Name < b.Name
	})
}
