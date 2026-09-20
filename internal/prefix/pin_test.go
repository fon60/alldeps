package prefix

import "testing"

func TestParsePin(t *testing.T) {
	cases := []struct {
		pin  string
		want string
		ok   bool
	}{
		{"22", "22", true},
		{" v22.15 ", "22.15", true},
		{"v20.19.1", "20.19.1", true},
		{"24.1.0", "24.1.0", true},
		{"lts/*", "", false},
		{"node", "", false},
		{"system", "", false},
		{"", "", false},
		{"   ", "", false},
		{"v", "", false},
		{"1.2.3.4", "", false},
		{"22.x", "", false},
		{"22..0", "", false},
	}
	for _, tc := range cases {
		if got, ok := ParsePin(tc.pin); ok != tc.ok || got != tc.want {
			t.Errorf("ParsePin(%q) = (%q, %v), want (%q, %v)", tc.pin, got, ok, tc.want, tc.ok)
		}
	}
}

func TestMatchPin(t *testing.T) {
	versions := []string{"v20.19.1", "v20.3.0", "v22.15.0"}
	cases := []struct {
		pin  string
		want string
	}{
		{"20.19.1", "v20.19.1"}, // full pin: exact match only
		{"v20.3.0", "v20.3.0"},
		{"22", "v22.15.0"},      // partial: highest installed with the dot-prefix
		{"20", "v20.19.1"},      // 20.19.1 > 20.3.0
		{"20.3", "v20.3.0"},
		{"22.15", "v22.15.0"},
		{"24", ""},              // unmatched
		{"20.99", ""},           // unmatched
		{"20.1", ""},            // no 20.1.x installed
		{"20.19.2", ""},         // full pin, not the exact installed version
		{"lts/*", ""},           // unparseable alias
		{"node", ""},            // unparseable alias
		{"", ""},                // empty
	}
	for _, tc := range cases {
		if got := MatchPin(tc.pin, versions); got != tc.want {
			t.Errorf("MatchPin(%q) = %q, want %q", tc.pin, got, tc.want)
		}
	}
}

func TestMatchPinEdgeCases(t *testing.T) {
	if got := MatchPin("22", []string{"bogus", "22.15.0"}); got != "22.15.0" {
		t.Errorf("unparseable installed entries must be skipped, got %q", got)
	}
	if got := MatchPin("v22", []string{"v22.4.0", "v22.15.0"}); got != "v22.15.0" {
		t.Errorf("leading v on the pin must match, got %q", got)
	}
	if got := MatchPin("22.15.0", []string{"v22.15.0-beta.1"}); got != "" {
		t.Errorf("a prerelease build is not an exact full-pin match, got %q", got)
	}
	if got := MatchPin("22", nil); got != "" {
		t.Errorf("no installed versions must yield no match, got %q", got)
	}
}
