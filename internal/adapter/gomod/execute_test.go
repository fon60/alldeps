package gomod

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fon60/alldeps/internal/ecosystem"
)

func TestExecuteGoGetArgvAndCwdPerOpKind(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(fixture, []byte(listFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	logFile := fakeGoShim(t, fixture)

	root := t.TempDir()
	e := NewProject(root)
	env := ecosystem.Environment{ID: root}

	cases := []struct {
		name     string
		batch    ecosystem.Batch
		wantArgs []string
	}{
		{
			name: "install pinned and unpinned",
			batch: ecosystem.Batch{Op: ecosystem.OpInstall, Items: []ecosystem.Item{
				{Name: "github.com/direct/alpha", Version: "v1.2.3"},
				{Name: "github.com/direct/fresh"},
			}},
			wantArgs: []string{"get", "github.com/direct/alpha@v1.2.3", "github.com/direct/fresh@latest"},
		},
		{
			name:     "upgrade to target version",
			batch:    ecosystem.Batch{Op: ecosystem.OpUpgrade, Items: []ecosystem.Item{{Name: "github.com/direct/alpha", Version: "v1.4.2"}}},
			wantArgs: []string{"get", "github.com/direct/alpha@v1.4.2"},
		},
		{
			name:     "remove via @none",
			batch:    ecosystem.Batch{Op: ecosystem.OpRemove, Items: []ecosystem.Item{{Name: "github.com/direct/beta"}}},
			wantArgs: []string{"get", "github.com/direct/beta@none"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.Execute(context.Background(), env, tc.batch); err != nil {
				t.Fatal(err)
			}
		})
	}

	invs := readGoInvocations(t, logFile)
	if len(invs) != len(cases) {
		t.Fatalf("got %d go invocations, want %d", len(invs), len(cases))
	}
	for i, tc := range cases {
		inv := invs[i]
		if inv.cwd != root {
			t.Fatalf("%s: cwd = %q, want the project dir %q", tc.name, inv.cwd, root)
		}
		if len(inv.args) != len(tc.wantArgs) {
			t.Fatalf("%s: argv = %v, want %v", tc.name, inv.args, tc.wantArgs)
		}
		for j := range tc.wantArgs {
			if inv.args[j] != tc.wantArgs[j] {
				t.Fatalf("%s: argv[%d] = %q, want %q (full argv %v)", tc.name, j, inv.args[j], tc.wantArgs[j], inv.args)
			}
		}
	}
}

func TestResolveBatchesPerOpKindWithGoGetLabels(t *testing.T) {
	e := NewProject(t.TempDir())
	plan, conflicts, err := e.Resolve(ecosystem.Intent{
		Env: ecosystem.Environment{ID: "/proj"},
		Items: []ecosystem.MarkedItem{
			{Op: ecosystem.OpInstall, Name: "github.com/a/b", Version: "v1.0.0"},
			{Op: ecosystem.OpUpgrade, Name: "github.com/c/d", Version: "v2.1.0"},
			{Op: ecosystem.OpRemove, Name: "github.com/e/f"},
			{Op: ecosystem.OpInstall, Name: "github.com/g/h"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %v, want none for Go", conflicts)
	}
	if len(plan.Batches) != 3 {
		t.Fatalf("batches = %+v, want one per op kind", plan.Batches)
	}
	wantLabels := map[ecosystem.OpKind]string{
		ecosystem.OpInstall: "go get github.com/a/b@v1.0.0 github.com/g/h@latest",
		ecosystem.OpUpgrade: "go get github.com/c/d@v2.1.0",
		ecosystem.OpRemove:  "go get github.com/e/f@none",
	}
	for _, b := range plan.Batches {
		if wantLabels[b.Op] != b.Label {
			t.Fatalf("label for %v = %q, want %q", b.Op, b.Label, wantLabels[b.Op])
		}
	}
}
