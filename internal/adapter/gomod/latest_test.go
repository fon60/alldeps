package gomod

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fon60/alldeps/internal/ecosystem"
)

const updateFixture = `{"Path":"example.com/fixture","Version":"(devel)","Main":true}
{"Path":"github.com/direct/alpha","Version":"v1.2.0","Update":{"Path":"github.com/direct/alpha","Version":"v1.4.2"}}
{"Path":"github.com/direct/beta","Version":"v0.3.1"}
{"Path":"golang.org/x/indirect","Version":"v0.4.0","Indirect":true,"Update":{"Path":"golang.org/x/indirect","Version":"v0.5.0"}}
`

func TestLatestVersionsFromToolchainUpdates(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(fixture, []byte(updateFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	logFile := fakeGoShim(t, fixture)

	root := t.TempDir()
	e := NewProject(root)
	versions, failed, err := e.LatestVersions(context.Background(), ecosystem.Environment{ID: root},
		[]string{"github.com/direct/alpha", "github.com/direct/beta"})
	if err != nil {
		t.Fatal(err)
	}
	if failed != 0 {
		t.Fatalf("failed = %d, want 0", failed)
	}
	if len(versions) != 1 || versions["github.com/direct/alpha"] != "v1.4.2" {
		t.Fatalf("candidates = %v, want only alpha -> v1.4.2 (up-to-date and indirect excluded)", versions)
	}

	invs := readGoInvocations(t, logFile)
	if len(invs) != 1 {
		t.Fatalf("got %d go invocations, want 1", len(invs))
	}
	wantArgs := []string{"list", "-m", "-u", "-json", "all"}
	if len(invs[0].args) != len(wantArgs) {
		t.Fatalf("argv = %v, want %v", invs[0].args, wantArgs)
	}
	for i := range wantArgs {
		if invs[0].args[i] != wantArgs[i] {
			t.Fatalf("argv[%d] = %q, want %q (full argv %v)", i, invs[0].args[i], wantArgs[i], invs[0].args)
		}
	}
}

func TestLatestVersionsTTLCache(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(fixture, []byte(updateFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	logFile := fakeGoShim(t, fixture)

	root := t.TempDir()
	e := NewProject(root)
	env := ecosystem.Environment{ID: root}
	names := []string{"github.com/direct/alpha"}
	if _, _, err := e.LatestVersions(context.Background(), env, names); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.LatestVersions(context.Background(), env, names); err != nil {
		t.Fatal(err)
	}
	if invs := readGoInvocations(t, logFile); len(invs) != 1 {
		t.Fatalf("go list -u ran %d times, want 1 (TTL cache should serve the second call)", len(invs))
	}

	e.TTL = -time.Second // force expiry
	if _, _, err := e.LatestVersions(context.Background(), env, names); err != nil {
		t.Fatal(err)
	}
	if invs := readGoInvocations(t, logFile); len(invs) != 2 {
		t.Fatalf("go list -u ran %d times after TTL expiry, want 2", len(invs))
	}
}
