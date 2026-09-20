package composer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"npmitude/internal/ecosystem"
)

// recordedRootClash is real composer 2.9 solver output: a pending root
// requirement whose candidates all need symfony/http-kernel ^6.4 while the
// project already requires ^7.0.
const recordedRootClash = `Your requirements could not be resolved to an installable set of packages.

  Problem 1
    - Root composer.json requires symfony/framework-bundle ^6.4 -> satisfiable by symfony/framework-bundle[v6.4.0, ..., v6.4.46].
    - symfony/framework-bundle[v6.4.0, ..., v6.4.46] require symfony/http-kernel ^6.4 -> found symfony/http-kernel[v6.4.0, ..., v6.4.46] but it conflicts with your root composer.json require (^7.0).

Use the option --with-all-dependencies (-W) to allow upgrades, downgrades and removals for packages currently locked to specific versions.

Installation failed, reverting ./composer.json and ./composer.lock to their original content.
`

// recordedImpossibleConstraint is real solver output for a constraint no
// released version matches.
const recordedImpossibleConstraint = `Your requirements could not be resolved to an installable set of packages.

  Problem 1
    - Root composer.json requires psr/log ^99, found psr/log[dev-master, dev-add-funding, 1.0.0, ..., 1.1.4, 2.0.0, 3.0.0, 3.0.1, 3.0.2, 3.x-dev (alias of dev-master)] but it does not match the constraint.

Use the option --with-all-dependencies (-W) to allow upgrades, downgrades and removals for packages currently locked to specific versions.

Installation failed, reverting ./composer.json and ./composer.lock to their original content.
`

// recordedPinnedPlatformMismatch is real solver output for a probe that pins
// the exact latest version: the platform line names "name vX.Y.Z" instead of
// "name[versions]" and reads "requires php".
const recordedPinnedPlatformMismatch = `Your requirements could not be resolved to an installable set of packages.

  Problem 1
    - Root composer.json requires symfony/console 8.1.7 -> satisfiable by symfony/console[v8.1.7].
    - symfony/console v8.1.7 requires php >=8.4.1 -> your php version (8.3.33) does not satisfy that requirement.


Installation failed, reverting ./composer.json and ./composer.lock to their original content.
`

// recordedNotFound is real composer 2.9 package-discovery output (the failure
// shape when the name itself does not exist).
const recordedNotFound = `In PackageDiscoveryTrait.php line 383:
  Could not find a matching version of package totally/nonexistent-xyz123. Ch
  eck the package spelling, your version constraint and that the package is a
  vailable in a stability which matches your minimum-stability (stable).
`

// recordedUnrecognized is solver-shaped output no class recognizes: it must
// degrade to the fallback conflict carrying the raw text.
const recordedUnrecognized = `Your requirements could not be resolved to an installable set of packages.

  Problem 1
    - the solver exploded while juggling version lattices and gave up without naming a package

Installation failed, reverting ./composer.json and ./composer.lock to their original content.
`

// recordedMarkerNoBlocks has the solver marker but no parseable Problem block.
const recordedMarkerNoBlocks = `Your requirements could not be resolved to an installable set of packages.

The dependency graph is in a state this composer build does not know how to explain.
`

func resolveWithFixture(t *testing.T, fixture, searchJSON, metaJSON string, items []ecosystem.MarkedItem) ([]ecosystem.Conflict, error) {
	t.Helper()
	fakeComposerProbeShim(t)
	path := filepath.Join(t.TempDir(), "solver.txt")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPOSER_PROBE_MODE", "solver-fail")
	t.Setenv("COMPOSER_PROBE_OUTPUT", path)
	pkg, _ := testPackagist(t, searchJSON, metaJSON)
	e := NewProject(t.TempDir())
	e.packagist = pkg
	conflicts, err := func() ([]ecosystem.Conflict, error) {
		plan, c, err := e.Resolve(ecosystem.Intent{Env: ecosystem.Environment{ID: e.root}, Items: items})
		if err != nil {
			return nil, err
		}
		if len(plan.Batches) != 0 {
			t.Fatalf("a failed probe must not contribute plan batches: %+v", plan.Batches)
		}
		return c, nil
	}()
	return conflicts, err
}

func sameEffect(got, want ecosystem.ResolutionEffect) bool {
	if got.Kind != want.Kind || got.Name != want.Name || got.TargetVersion != want.TargetVersion {
		return false
	}
	if len(got.Destinations) != len(want.Destinations) {
		return false
	}
	for i := range got.Destinations {
		if got.Destinations[i] != want.Destinations[i] {
			return false
		}
	}
	return true
}

func TestClassifySolverFailures(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		searchJSON  string
		metaJSON    string
		items       []ecosystem.MarkedItem
		wantPkg     string
		wantEffects []ecosystem.ResolutionEffect
		wantMsgPart string
	}{
		{
			name: "platform mismatch pins last compatible release line and offers skip",
			fixture: recordedPlatformMismatch,
			metaJSON: `{"packages":{"symfony/console":[
				{"name":"symfony/console","version":"8.1.7","require":{"php":">=8.4.1"}},
				{"version":"8.0.15"},
				{"version":"7.4.19","require":{"php":">=8.2"}},
				{"version":"7.4.18"}
			]}}`,
			items:   []ecosystem.MarkedItem{{Op: ecosystem.OpInstall, Name: "symfony/console", Version: "^8.0"}},
			wantPkg: "symfony/console",
			wantEffects: []ecosystem.ResolutionEffect{
				{Kind: "install", Name: "symfony/console", TargetVersion: "7.4.19"},
				{Kind: "skip", Name: "symfony/console"},
			},
			wantMsgPart: "does not satisfy that requirement",
		},
		{
			name: "a pinned latest version that mismatches the platform still classifies as platform, not root-clash",
			fixture: recordedPinnedPlatformMismatch,
			metaJSON: `{"packages":{"symfony/console":[
				{"name":"symfony/console","version":"8.1.7","require":{"php":">=8.4.1"}},
				{"version":"8.0.15"},
				{"version":"7.4.19","require":{"php":">=8.2"}},
				{"version":"7.4.18"}
			]}}`,
			items:   []ecosystem.MarkedItem{{Op: ecosystem.OpInstall, Name: "symfony/console", Version: "8.1.7"}},
			wantPkg: "symfony/console",
			wantEffects: []ecosystem.ResolutionEffect{
				{Kind: "install", Name: "symfony/console", TargetVersion: "7.4.19"},
				{Kind: "skip", Name: "symfony/console"},
			},
			wantMsgPart: "does not satisfy that requirement",
		},
		{
			name:        "root-dep clash pins candidates, offers removing the clashing root dep and skip",
			fixture:     recordedRootClash,
			searchJSON:  `{}`,
			metaJSON:    `{}`,
			items:       []ecosystem.MarkedItem{{Op: ecosystem.OpInstall, Name: "symfony/framework-bundle", Version: "^6.4"}},
			wantPkg:     "symfony/framework-bundle",
			wantEffects: []ecosystem.ResolutionEffect{
				{Kind: "install", Name: "symfony/framework-bundle", TargetVersion: "v6.4.46"},
				{Kind: "install", Name: "symfony/http-kernel", TargetVersion: "^6.4"},
				{Kind: "remove", Name: "symfony/http-kernel"},
				{Kind: "skip", Name: "symfony/framework-bundle"},
			},
			wantMsgPart: "conflicts with your root composer.json require",
		},
		{
			name:        "impossible constraint pins the best named candidate and offers skip",
			fixture:     recordedImpossibleConstraint,
			searchJSON:  `{}`,
			metaJSON:    `{}`,
			items:       []ecosystem.MarkedItem{{Op: ecosystem.OpInstall, Name: "psr/log", Version: "^99"}},
			wantPkg:     "psr/log",
			wantEffects: []ecosystem.ResolutionEffect{
				{Kind: "install", Name: "psr/log", TargetVersion: "3.0.2"},
				{Kind: "skip", Name: "psr/log"},
			},
			wantMsgPart: "does not match the constraint",
		},
		{
			name:        "not found offers the corrected Packagist name and skip",
			fixture:     recordedNotFound,
			searchJSON:  `{"results":[{"name":"totally/existent","description":"the real one"}],"total":1}`,
			metaJSON:    `{}`,
			items:       []ecosystem.MarkedItem{{Op: ecosystem.OpInstall, Name: "totally/nonexistent-xyz123"}},
			wantPkg:     "totally/nonexistent-xyz123",
			wantEffects: []ecosystem.ResolutionEffect{
				{Kind: "install", Name: "totally/existent"},
				{Kind: "skip", Name: "totally/nonexistent-xyz123"},
			},
			wantMsgPart: "could not be found",
		},
		{
			name:        "unrecognized problem degrades to the fallback with raw text and generic options",
			fixture:     recordedUnrecognized,
			searchJSON:  `{}`,
			metaJSON:    `{}`,
			items:       []ecosystem.MarkedItem{{Op: ecosystem.OpInstall, Name: "foo/bar", Version: "^1.0"}},
			wantPkg:     "foo/bar",
			wantEffects: []ecosystem.ResolutionEffect{
				{},
				{Kind: "skip", Name: "foo/bar"},
			},
			wantMsgPart: "juggling version lattices",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts, err := resolveWithFixture(t, tt.fixture, tt.searchJSON, tt.metaJSON, tt.items)
			if err != nil {
				t.Fatalf("Resolve returned an error for a classifiable failure: %v", err)
			}
			if len(conflicts) != 1 {
				t.Fatalf("conflicts = %+v, want exactly one", conflicts)
			}
			c := conflicts[0]
			if c.Package != tt.wantPkg {
				t.Fatalf("conflict package = %q, want %q", c.Package, tt.wantPkg)
			}
			if !strings.Contains(c.Message, tt.wantMsgPart) {
				t.Fatalf("message = %q, want it to carry the solver text (part %q)", c.Message, tt.wantMsgPart)
			}
			if len(c.Options) != len(tt.wantEffects) {
				t.Fatalf("options = %+v, want %d options matching %v", c.Options, len(tt.wantEffects), tt.wantEffects)
			}
			for i, want := range tt.wantEffects {
				got := c.Options[i].Effect
				if !sameEffect(got, want) {
					t.Fatalf("option[%d] effect = %+v (label %q), want %+v", i, got, c.Options[i].Label, want)
				}
			}
			for _, o := range c.Options {
				if o.Label == "" || o.Description == "" {
					t.Fatalf("every option must carry a label and a stated consequence: %+v", o)
				}
			}
		})
	}
}

func TestClassifyUnparseableSolverFailureAttributesEveryPendingItem(t *testing.T) {
	conflicts, err := resolveWithFixture(t, recordedMarkerNoBlocks, `{}`, `{}`, []ecosystem.MarkedItem{
		{Op: ecosystem.OpInstall, Name: "foo/bar"},
		{Op: ecosystem.OpInstall, Name: "baz/qux"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 2 {
		t.Fatalf("conflicts = %+v, want one per pending item", conflicts)
	}
	for i, name := range []string{"foo/bar", "baz/qux"} {
		if conflicts[i].Package != name {
			t.Fatalf("conflict[%d] package = %q, want %q", i, conflicts[i].Package, name)
		}
		if len(conflicts[i].Options) != 2 || conflicts[i].Options[1].Effect.Kind != "skip" {
			t.Fatalf("fallback options = %+v, want choose-version + skip", conflicts[i].Options)
		}
	}
}

func TestBestCandidatePrefersHighestStable(t *testing.T) {
	if got := bestCandidate("v6.4.0, ..., v6.4.46"); got != "v6.4.46" {
		t.Fatalf("bestCandidate = %q, want v6.4.46", got)
	}
	if got := bestCandidate("dev-master, 1.0.0, 3.x-dev (alias of dev-master)"); got != "1.0.0" {
		t.Fatalf("bestCandidate = %q, want the only stable 1.0.0", got)
	}
	if got := bestCandidate("dev-main, 9999999-dev"); got != "9999999-dev" {
		t.Fatalf("bestCandidate = %q, want the highest unstable name when no stable one exists (the pin must stay machine-applicable)", got)
	}
}

func TestPhpSatisfiesConstraintForms(t *testing.T) {
	cases := []struct {
		constraint string
		version    string
		want       bool
	}{
		{">=8.4", "8.3.33", false},
		{">=8.4.1", "8.3.33", false},
		{">=8.2", "8.3.33", true},
		{"^8.1", "8.3.33", true},
		{"^8.4", "8.3.33", false},
		{"~8.3.0", "8.3.33", true},
		{"~8.3.0", "8.4.0", false},
		{">=7.2.5 || ^8.0", "8.3.33", true},
		{"^7.2.5 || ^8.0", "7.4.33", true},
		{"^7.2.5 || ^8.0", "9.0.0", false},
		{"8.1.*", "8.1.27", true},
		{"8.1.*", "8.2.0", false},
		{"*", "8.3.33", true},
		{"8.1", "8.1.99", false},
		{">=8.1 <8.4", "8.3.33", true},
		{">=8.1 <8.4", "8.4.0", false},
	}
	for _, c := range cases {
		if got := phpSatisfies(c.constraint, c.version); got != c.want {
			t.Errorf("phpSatisfies(%q, %q) = %v, want %v", c.constraint, c.version, got, c.want)
		}
	}
}
