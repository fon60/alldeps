package filter

import "testing"

var (
	installedCurrent = View("alpha", true, false, false, false)
	installedUpgrad  = View("beta", true, true, false, false)
	notInstalled     = View("gamma", false, false, false, false)
	brokenInstalled  = View("delta", true, false, true, false)
	conflictedRow    = View("epsilon", true, false, false, true)
	namePrettier     = View("prettier", true, false, false, false)
	namePrettierPlug = View("prettier-plugin-x", true, false, false, false)
	nameXprettier    = View("xprettier", true, false, false, false)
)

func eval(t *testing.T, expr string, views ...PkgView) []bool {
	t.Helper()
	pred, err := Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q): %v", expr, err)
	}
	out := make([]bool, len(views))
	for i, v := range views {
		out[i] = pred(v)
	}
	return out
}

func TestSimpleTests(t *testing.T) {
	if got := eval(t, "~i", installedCurrent, notInstalled, brokenInstalled); got[0] != true || got[1] != false || got[2] != true {
		t.Fatalf("~i = %v", got)
	}
	if got := eval(t, "~u", installedCurrent, installedUpgrad, notInstalled); got[0] != false || got[1] != true || got[2] != false {
		t.Fatalf("~u = %v", got)
	}
	if got := eval(t, "~b", installedCurrent, brokenInstalled); got[0] != false || got[1] != true {
		t.Fatalf("~b = %v", got)
	}
	if got := eval(t, "~c", installedCurrent, conflictedRow); got[0] != false || got[1] != true {
		t.Fatalf("~c = %v", got)
	}
}

func TestNamePatterns(t *testing.T) {
	got := eval(t, `~n ^prettier`, namePrettier, namePrettierPlug, nameXprettier)
	if got[0] != true || got[1] != true || got[2] != false {
		t.Fatalf("~n ^prettier = %v", got)
	}
	got = eval(t, `~n prettier`, namePrettier, nameXprettier)
	if got[0] != true || got[1] != true {
		t.Fatalf("~n prettier (substring) = %v", got)
	}
	got = eval(t, `~n ^alpha$`, installedCurrent)
	if got[0] != true {
		t.Fatalf("exact match failed: %v", got)
	}
}

func TestCombinations(t *testing.T) {
	// installed AND upgradable
	got := eval(t, "~i & ~u", installedCurrent, installedUpgrad, notInstalled)
	if got[0] != false || got[1] != true || got[2] != false {
		t.Fatalf("~i & ~u = %v", got)
	}
	// installed OR broken
	got = eval(t, "~i | ~b", installedCurrent, notInstalled, brokenInstalled)
	if got[0] != true || got[1] != false || got[2] != true {
		t.Fatalf("~i | ~b = %v", got)
	}
	// negation
	got = eval(t, "!~b", installedCurrent, brokenInstalled)
	if got[0] != true || got[1] != false {
		t.Fatalf("!~b = %v", got)
	}
	// precedence: ! binds tighter than &, & tighter than |
	got = eval(t, "~b | ~i & ~u", installedCurrent, installedUpgrad, brokenInstalled, notInstalled)
	if got[0] != false || got[1] != true || got[2] != true || got[3] != false {
		t.Fatalf("precedence = %v", got)
	}
	// parentheses override precedence
	got = eval(t, "(~b | ~i) & ~u", installedCurrent, installedUpgrad, brokenInstalled)
	if got[0] != false || got[1] != true || got[2] != false {
		t.Fatalf("parens = %v", got)
	}
	// double negation
	got = eval(t, "!!~i", installedCurrent, notInstalled)
	if got[0] != true || got[1] != false {
		t.Fatalf("!!~i = %v", got)
	}
	// whitespace tolerance
	got = eval(t, "  ~i  &  ~u  ", installedUpgrad)
	if got[0] != true {
		t.Fatalf("whitespace = %v", got)
	}
	// name pattern combined with tests
	got = eval(t, `~n ^prettier & ~i`, namePrettier, nameXprettier)
	if got[0] != true || got[1] != false {
		t.Fatalf("combined name = %v", got)
	}
}

func TestEmptyMatchesAll(t *testing.T) {
	got := eval(t, "", installedCurrent, notInstalled, brokenInstalled)
	for i, b := range got {
		if !b {
			t.Fatalf("empty expression should match all, index %d = %v", i, b)
		}
	}
}

func TestInvalidExpressionsRejected(t *testing.T) {
	invalid := []string{
		"~z",           // unknown test
		"~n",           // missing pattern
		"~n (unclosed", // unbalanced paren inside pattern? actually '(' starts group... use below
		"(~i",          // missing closing paren
		"~i ~i",        // two primaries without operator
		"& ~i",         // leading &
		"~i |",         // dangling |
		"! ",           // dangling !
		`~n (`,         // empty pattern due to group start... '(' is not allowed in pattern -> pattern empty? handled: '(' breaks loop immediately -> empty pattern error
		"()",           // empty group
		`~n [`,         // invalid regex
	}
	for _, expr := range invalid {
		if _, err := Parse(expr); err == nil {
			t.Errorf("Parse(%q) should fail", expr)
		}
	}
}

func TestInvalidRegexRejected(t *testing.T) {
	if _, err := Parse(`~n [unclosed`); err == nil {
		t.Fatal("invalid regex pattern should be rejected at parse time")
	}
}
