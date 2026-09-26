package gomod

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"npmitude/internal/ecosystem"
)

const listFixture = `{"Path":"example.com/fixture","Version":"(devel)","Main":true}
{"Path":"github.com/direct/alpha","Version":"v1.2.0"}
{"Path":"github.com/direct/beta","Version":"v0.3.1"}
{"Path":"golang.org/x/indirect","Version":"v0.4.0","Indirect":true}
{"Path":"github.com/other/gamma","Version":"v2.0.0","Indirect":true}
`

func TestListInstalledFullBuildListWithResolvedVersions(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(fixture, []byte(listFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	logFile := fakeGoShim(t, fixture)

	root := t.TempDir()
	e := NewProject(root)
	pkgs, err := e.ListInstalled(context.Background(), ecosystem.Environment{ID: root})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ecosystem.Package{}
	for _, p := range pkgs {
		got[p.Name] = p
	}
	want := map[string]struct {
		version   string
		automatic bool
	}{
		"github.com/direct/alpha":    {"v1.2.0", false},
		"github.com/direct/beta":     {"v0.3.1", false},
		"golang.org/x/indirect":      {"v0.4.0", true},
		"github.com/other/gamma":     {"v2.0.0", true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d packages %v, want exactly %d (indirect listed, main excluded)", len(got), got, len(want))
	}
	for name, w := range want {
		p, ok := got[name]
		if !ok {
			t.Fatalf("%s missing from the list", name)
		}
		if p.Version != w.version {
			t.Fatalf("%s version = %q, want resolved %q", name, p.Version, w.version)
		}
		if p.Automatic != w.automatic {
			t.Fatalf("%s automatic = %v, want %v", name, p.Automatic, w.automatic)
		}
	}
	if _, ok := got["example.com/fixture"]; ok {
		t.Fatal("the main module must not be listed")
	}

	invs := readGoInvocations(t, logFile)
	if len(invs) != 1 {
		t.Fatalf("got %d go invocations, want 1", len(invs))
	}
	inv := invs[0]
	if inv.cwd != root {
		t.Fatalf("go list cwd = %q, want the project dir %q", inv.cwd, root)
	}
	wantArgs := []string{"list", "-m", "-json", "all"}
	if len(inv.args) != len(wantArgs) {
		t.Fatalf("argv = %v, want %v", inv.args, wantArgs)
	}
	for i := range wantArgs {
		if inv.args[i] != wantArgs[i] {
			t.Fatalf("argv[%d] = %q, want %q (full argv %v)", i, inv.args[i], wantArgs[i], inv.args)
		}
	}
}

func TestListInstalledParseFailureIsLoadFailure(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(fixture, []byte("not json at all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeGoShim(t, fixture)

	root := t.TempDir()
	pkgs, err := NewProject(root).ListInstalled(context.Background(), ecosystem.Environment{ID: root})
	if err == nil {
		t.Fatalf("expected a load failure on unparseable output, got %v packages", pkgs)
	}
}
