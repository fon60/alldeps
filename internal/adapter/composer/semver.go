package composer

import (
	"strconv"
	"strings"
)

// phpSatisfies reports whether a PHP version string satisfies a composer
// constraint — best-effort over the forms that appear in platform
// requirements: *, exact, X.Y.* wildcards, ^, ~, comparisons, comma/space AND
// and || OR.
func phpSatisfies(constraint, version string) bool {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "*" {
		return true
	}
	for _, alt := range strings.Split(constraint, "||") {
		ok := true
		for _, term := range strings.FieldsFunc(alt, func(r rune) bool { return r == ' ' || r == ',' }) {
			if !termSatisfies(term, version) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func termSatisfies(term, version string) bool {
	term = strings.TrimSpace(term)
	for _, op := range []string{">=", "<=", "!=", "^", "~", ">", "<", "="} {
		if !strings.HasPrefix(term, op) {
			continue
		}
		target := strings.TrimSpace(strings.TrimPrefix(term, op))
		switch op {
		case ">=":
			return compareVersions(version, target) >= 0
		case "<=":
			return compareVersions(version, target) <= 0
		case ">":
			return compareVersions(version, target) > 0
		case "<":
			return compareVersions(version, target) < 0
		case "!=":
			return compareVersions(version, target) != 0
		case "=":
			return compareVersions(version, target) == 0
		case "^":
			return caretSatisfies(version, target)
		case "~":
			return tildeSatisfies(version, target)
		}
	}
	if strings.HasSuffix(term, ".*") {
		prefix := splitVersion(strings.TrimSuffix(term, ".*"))
		vs := splitVersion(version)
		if len(vs) < len(prefix) {
			return false
		}
		for i, pseg := range prefix {
			if vs[i] != pseg {
				return false
			}
		}
		return true
	}
	return compareVersions(version, term) == 0
}

func caretSatisfies(version, target string) bool {
	if compareVersions(version, target) < 0 {
		return false
	}
	segs := splitVersion(target)
	for i, s := range segs {
		if s != 0 {
			segs[i]++
			for j := i + 1; j < len(segs); j++ {
				segs[j] = 0
			}
			return compareVersions(version, joinVersion(segs)) < 0
		}
	}
	return false
}

func tildeSatisfies(version, target string) bool {
	if compareVersions(version, target) < 0 {
		return false
	}
	segs := splitVersion(target)
	switch len(segs) {
	case 1:
		segs[0]++
	case 2:
		segs[0]++
	default:
		segs[len(segs)-2]++
		segs[len(segs)-1] = 0
	}
	return compareVersions(version, joinVersion(segs)) < 0
}

func splitVersion(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n := 0
		i := 0
		for i < len(p) && p[i] >= '0' && p[i] <= '9' {
			n = n*10 + int(p[i]-'0')
			i++
		}
		if i == 0 {
			break
		}
		out = append(out, n)
	}
	return out
}

func joinVersion(segs []int) string {
	parts := make([]string, len(segs))
	for i, s := range segs {
		parts[i] = strconv.Itoa(s)
	}
	return strings.Join(parts, ".")
}

// compareVersions orders dotted version strings numerically; missing segments
// count as zero ("8.3" < "8.3.33").
func compareVersions(a, b string) int {
	as, bs := splitVersion(a), splitVersion(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		switch {
		case av < bv:
			return -1
		case av > bv:
			return 1
		}
	}
	return 0
}
