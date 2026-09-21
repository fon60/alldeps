package gomod

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"npmitude/internal/ecosystem"
)

const versionsFixture = "github.com/direct/alpha v1.0.0 v1.2.3 v2.0.0-beta v2.0.0\n"

func TestDocEnumeratesModuleVersions(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "versions.txt")
	if err := os.WriteFile(fixture, []byte(versionsFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	logFile := fakeGoShim(t, fixture)

	root := t.TempDir()
	e := NewProject(root)
	doc, local, err := e.Doc(context.Background(), ecosystem.Environment{ID: root}, "github.com/direct/alpha", false)
	if err != nil {
		t.Fatal(err)
	}
	if local {
		t.Fatal("a go module doc must not be reported as a local doc")
	}
	if doc.Name != "github.com/direct/alpha" {
		t.Fatalf("name = %q, want the module path", doc.Name)
	}
	want := []string{"v2.0.0", "v2.0.0-beta", "v1.2.3", "v1.0.0"}
	if len(doc.Versions) != len(want) {
		t.Fatalf("versions = %v, want %v (newest first)", doc.Versions, want)
	}
	for i := range want {
		if doc.Versions[i] != want[i] {
			t.Fatalf("versions[%d] = %q, want %q (full %v)", i, doc.Versions[i], want[i], doc.Versions)
		}
	}
	if doc.Latest != "v2.0.0" {
		t.Fatalf("latest = %q, want the newest tag v2.0.0", doc.Latest)
	}

	invs := readGoInvocations(t, logFile)
	if len(invs) != 1 {
		t.Fatalf("got %d go invocations, want 1", len(invs))
	}
	wantArgs := []string{"list", "-m", "-versions", "github.com/direct/alpha"}
	if len(invs[0].args) != len(wantArgs) {
		t.Fatalf("argv = %v, want %v", invs[0].args, wantArgs)
	}
	for i := range wantArgs {
		if invs[0].args[i] != wantArgs[i] {
			t.Fatalf("argv[%d] = %q, want %q (full argv %v)", i, invs[0].args[i], wantArgs[i], invs[0].args)
		}
	}
	if invs[0].cwd != root {
		t.Fatalf("cwd = %q, want the project root", invs[0].cwd)
	}
}

func TestDocVersionsFailureIsAnError(t *testing.T) {
	bindir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = "list" ] && [ "$3" = "-versions" ]; then
  echo "go: module lookup disabled by GOFLAGS" >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(bindir, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	e := NewProject(root)
	doc, _, err := e.Doc(context.Background(), ecosystem.Environment{ID: root}, "github.com/direct/alpha", false)
	if err == nil {
		t.Fatal("Doc must fail when go list -m -versions fails")
	}
	if doc != nil {
		t.Fatalf("no doc may be fabricated on failure: %+v", doc)
	}
	if !strings.Contains(err.Error(), "go list -m -versions") {
		t.Fatalf("error should name the failing invocation, got %v", err)
	}
	if !strings.Contains(err.Error(), "module lookup disabled") {
		t.Fatalf("error should carry the toolchain stderr, got %v", err)
	}
}
