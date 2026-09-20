package prefix

import (
	"strconv"
	"strings"

	"npmitude/internal/domain"
)

// ParsePin normalizes a .nvmrc pin: trims whitespace, strips one leading "v",
// and requires 1-3 dot-separated numeric components. It returns the canonical
// "major[.minor[.patch]]" form and whether the pin parsed at all; aliases
// (lts/*, node, …) and empty pins do not.
func ParsePin(pin string) (string, bool) {
	comps := splitNumeric(stripV(strings.TrimSpace(pin)))
	if comps == nil || len(comps) > 3 {
		return "", false
	}
	parts := make([]string, len(comps))
	for i, n := range comps {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, "."), true
}

// MatchPin resolves a .nvmrc pin against installed Node versions: a full
// (3-component) pin matches only the exactly installed version; a partial pin
// (1-2 components) matches the highest installed version having that
// dot-prefix. It returns "" when the pin is unparseable or nothing matches.
func MatchPin(pin string, versions []string) string {
	p, ok := ParsePin(pin)
	if !ok {
		return ""
	}
	var best string
	for _, v := range versions {
		if !pinMatches(p, v) {
			continue
		}
		if best == "" || domain.CompareVersions(v, best) > 0 {
			best = v
		}
	}
	return best
}

func pinMatches(pin, version string) bool {
	pc := splitNumeric(pin)
	vc := splitNumeric(stripV(version))
	if pc == nil || vc == nil {
		return false
	}
	if len(pc) == 3 {
		if len(vc) != 3 {
			return false
		}
	} else if len(vc) < len(pc) {
		return false
	}
	for i, n := range pc {
		if vc[i] != n {
			return false
		}
	}
	return true
}

func stripV(v string) string {
	v = strings.TrimSpace(v)
	if s, ok := strings.CutPrefix(v, "v"); ok {
		v = s
	}
	return v
}

// splitNumeric splits a dotted version core into numeric components, or nil
// when any component is not a plain non-negative integer.
func splitNumeric(s string) []int {
	if s == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(s, ".") {
		if part == "" {
			return nil
		}
		n := 0
		for _, c := range part {
			if c < '0' || c > '9' {
				return nil
			}
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
	}
	return out
}
