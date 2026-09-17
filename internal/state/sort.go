package state

import (
	"sort"
	"strconv"
	"strings"
)

// CompareVersions compares two npm/semver-style version strings.
// Returns -1, 0, or 1. Missing numeric segments count as 0; a version with a
// prerelease tag sorts before the same version without one.
func CompareVersions(a, b string) int {
	a = strings.TrimPrefix(strings.TrimSpace(a), "v")
	b = strings.TrimPrefix(strings.TrimSpace(b), "v")

	aCore, aPre := splitPre(a)
	bCore, bPre := splitPre(b)

	if c := compareCore(aCore, bCore); c != 0 {
		return c
	}
	switch {
	case aPre == "" && bPre == "":
		return 0
	case aPre == "":
		return 1
	case bPre == "":
		return -1
	default:
		return comparePre(aPre, bPre)
	}
}

func splitPre(v string) (core, pre string) {
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

func compareCore(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(as) {
			av, _ = strconv.Atoi(leadingInt(as[i]))
		}
		if i < len(bs) {
			bv, _ = strconv.Atoi(leadingInt(bs[i]))
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

func leadingInt(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return "0"
	}
	return s[:i]
}

func comparePre(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		aok, bok := aerr == nil, berr == nil
		switch {
		case aok && bok:
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
		case aok:
			return -1 // numeric identifiers sort before alphanumeric
		case bok:
			return 1
		default:
			if as[i] != bs[i] {
				if as[i] < bs[i] {
					return -1
				}
				return 1
			}
		}
	}
	if len(as) != len(bs) {
		if len(as) < len(bs) {
			return -1
		}
		return 1
	}
	return 0
}

// SortVersions sorts version strings newest-first (descending semver).
func SortVersions(versions []string) {
	sort.SliceStable(versions, func(i, j int) bool {
		return CompareVersions(versions[i], versions[j]) > 0
	})
}

// SortRows sorts rows in place by key. Unknown sizes sort last for size
// sorting; the name order is the tie-breaker everywhere.
func SortRows(rows []*PkgState, key SortKey) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch key {
		case SortVersion:
			if c := CompareVersions(a.InstalledVersion, b.InstalledVersion); c != 0 {
				return c < 0
			}
		case SortSize:
			as, bs := a.SizeBytes, b.SizeBytes
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
			if fa, fb := a.Flag(), b.Flag(); fa != fb {
				return fa < fb
			}
		}
		return a.Name < b.Name
	})
}
