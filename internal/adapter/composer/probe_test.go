package composer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fon60/alldeps/internal/ecosystem"
)

// recordedPlatformMismatch is real composer 2.9 solver output for a pinned
// package whose php requirement the platform does not satisfy.
const recordedPlatformMismatch = `Composer could not detect the root package (scratch/proj) version, defaulting to '1.0.0'. See https://getcomposer.org/root-version
./composer.json has been updated
Running composer update symfony/console
Loading composer repositories with package information
Updating dependencies
Your requirements could not be resolved to an installable set of packages.

  Problem 1
    - Root composer.json requires symfony/console ^8.0 -> satisfiable by symfony/console[v8.0.0, ..., v8.1.7].
    - symfony/console[v8.0.0, ..., v8.0.15] require php >=8.4 -> your php version (8.3.33) does not satisfy that requirement.
    - symfony/console[v8.1.0, ..., v8.1.7] require php >=8.4.1 -> your php version (8.3.33) does not satisfy that requirement.


Installation failed, reverting ./composer.json and ./composer.lock to their original content.
`

// fakeComposerProbeShim puts a `composer` script first on PATH whose behavior
// is controlled by two env vars: COMPOSER_PROBE_MODE (success | solver-fail |
// other-fail) and, for the fail modes, COMPOSER_PROBE_OUTPUT naming a file to
// print. Every invocation is logged (cwd + argv) to the returned log file.
func fakeComposerProbeShim(t *testing.T) string {
	t.Helper()
	bindir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "composer.log")
	script := `#!/bin/sh
{
  printf 'cwd=%s\n' "$PWD"
  for a in "$@"; do printf 'arg=%s\n' "$a"; done
} >> "` + logFile + `"
mode="${COMPOSER_PROBE_MODE:-success}"
case "$mode" in
  success)
    echo "Using version ^1.0 for the requested package(s)"
    exit 0
    ;;
  solver-fail)
    cat "${COMPOSER_PROBE_OUTPUT:?}"
    exit 2
    ;;
  other-fail)
    echo "composer: some unrelated internal error"
    exit 1
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(bindir, "composer"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logFile
}

func TestResolveDryRunSuccessYieldsPlanWithoutConflicts(t *testing.T) {
	logFile := fakeComposerProbeShim(t)
	t.Setenv("COMPOSER_PROBE_MODE", "success")
	root := t.TempDir()

	plan, conflicts, err := NewProject(root).Resolve(ecosystem.Intent{
		Env:   ecosystem.Environment{ID: root},
		Items: []ecosystem.MarkedItem{
			{Op: ecosystem.OpInstall, Name: "foo/bar", Version: "^1.0"},
			{Op: ecosystem.OpRemove, Name: "baz/qux"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none on a passing probe", conflicts)
	}
	if len(plan.Batches) != 2 {
		t.Fatalf("plan batches = %d, want one per op kind: %+v", len(plan.Batches), plan.Batches)
	}
	if plan.Batches[0].Op != ecosystem.OpInstall || plan.Batches[1].Op != ecosystem.OpRemove {
		t.Fatalf("batch order = %v, %v, want install then remove", plan.Batches[0].Op, plan.Batches[1].Op)
	}

	invs := readComposerInvocations(t, logFile)
	if len(invs) != 2 {
		t.Fatalf("got %d invocations, want one dry-run probe per op kind", len(invs))
	}
	for _, inv := range invs {
		if inv.cwd != root {
			t.Fatalf("probe cwd = %q, want the project dir %q", inv.cwd, root)
		}
		joined := strings.Join(inv.args, " ")
		if !strings.Contains(joined, "--dry-run") || !strings.Contains(joined, "--no-interaction") {
			t.Fatalf("probe argv = %v, want --dry-run and --no-interaction", inv.args)
		}
	}
}

func TestResolveDryRunSolverFailureYieldsConflicts(t *testing.T) {
	logFile := fakeComposerProbeShim(t)
	fixture := filepath.Join(t.TempDir(), "solver.txt")
	if err := os.WriteFile(fixture, []byte(recordedPlatformMismatch), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPOSER_PROBE_MODE", "solver-fail")
	t.Setenv("COMPOSER_PROBE_OUTPUT", fixture)
	root := t.TempDir()

	// Hermetic Packagist stub: the pin option must come from this metadata,
	// not from the real index.
	meta := `{"packages":{"symfony/console":[
	  {"name":"symfony/console","version":"8.1.7","require":{"php":">=8.4.1"}},
	  {"version":"8.0.15"},
	  {"version":"7.4.19","require":{"php":">=8.2"}},
	  {"version":"7.4.18"}
	]}}`
	pkg, _ := testPackagist(t, `{}`, meta)
	e := NewProject(root)
	e.packagist = pkg

	plan, conflicts, err := e.Resolve(ecosystem.Intent{
		Env:   ecosystem.Environment{ID: root},
		Items: []ecosystem.MarkedItem{{Op: ecosystem.OpInstall, Name: "symfony/console", Version: "^8.0"}},
	})
	if err != nil {
		t.Fatalf("a solver failure must classify into conflicts, not an error: %v", err)
	}
	if len(conflicts) == 0 {
		t.Fatal("conflicts = empty, want the parsed solver problem")
	}
	if conflicts[0].Package != "symfony/console" {
		t.Fatalf("conflict package = %q, want the pending item", conflicts[0].Package)
	}
	if len(conflicts[0].Options) == 0 {
		t.Fatal("the conflict must carry resolution options")
	}
	if len(plan.Batches) != 0 {
		t.Fatalf("plan batches = %+v, want none when the probe failed", plan.Batches)
	}

	invs := readComposerInvocations(t, logFile)
	if len(invs) != 1 {
		t.Fatalf("got %d invocations, want exactly one probe", len(invs))
	}
	if !strings.Contains(strings.Join(invs[0].args, " "), "--dry-run") {
		t.Fatalf("probe argv = %v, want --dry-run", invs[0].args)
	}
}

func TestResolveDryRunUnrelatedFailureIsAnError(t *testing.T) {
	fakeComposerProbeShim(t)
	t.Setenv("COMPOSER_PROBE_MODE", "other-fail")
	root := t.TempDir()

	_, conflicts, err := NewProject(root).Resolve(ecosystem.Intent{
		Env:   ecosystem.Environment{ID: root},
		Items: []ecosystem.MarkedItem{{Op: ecosystem.OpInstall, Name: "foo/bar"}},
	})
	if err == nil {
		t.Fatal("an unclassifiable non-zero exit must surface as an error")
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none for a non-solver failure", conflicts)
	}
}
