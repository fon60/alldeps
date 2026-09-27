package composer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fon60/alldeps/internal/ecosystem"
)

// fakeComposerShim puts a `composer` script first on PATH that records every
// invocation (cwd plus one arg per line) to a log file and exits 0, echoing
// the recorded line count. It returns the log file path.
func fakeComposerShim(t *testing.T) string {
	t.Helper()
	bindir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "composer.log")
	script := `#!/bin/sh
{
  printf 'cwd=%s\n' "$PWD"
  for a in "$@"; do printf 'arg=%s\n' "$a"; done
} >> "` + logFile + `"
echo ok
`
	if err := os.WriteFile(filepath.Join(bindir, "composer"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logFile
}

type composerInvocation struct {
	cwd  string
	args []string
}

func readComposerInvocations(t *testing.T, logFile string) []composerInvocation {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("no composer invocations recorded: %v", err)
	}
	var invs []composerInvocation
	var cur *composerInvocation
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "cwd=") {
			invs = append(invs, composerInvocation{cwd: strings.TrimPrefix(line, "cwd=")})
			cur = &invs[len(invs)-1]
			continue
		}
		if strings.HasPrefix(line, "arg=") && cur != nil {
			cur.args = append(cur.args, strings.TrimPrefix(line, "arg="))
		}
	}
	return invs
}

func assertExactArgs(t *testing.T, inv composerInvocation, cwd string, want []string) {
	t.Helper()
	if inv.cwd != cwd {
		t.Fatalf("composer cwd = %q, want the project dir %q", inv.cwd, cwd)
	}
	if len(inv.args) != len(want) {
		t.Fatalf("argv = %v, want %v", inv.args, want)
	}
	for i := range want {
		if inv.args[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q (full argv %v)", i, inv.args[i], want[i], inv.args)
		}
	}
	for _, a := range inv.args {
		if strings.Contains(a, "--no-scripts") {
			t.Fatalf("argv must not disable lifecycle scripts: %v", inv.args)
		}
		if strings.Contains(a, "--dry-run") {
			t.Fatalf("Execute argv must not be a dry-run probe: %v", inv.args)
		}
	}
}

func TestExecuteInstallRequiresConstraintWithoutNoScripts(t *testing.T) {
	logFile := fakeComposerShim(t)
	root := t.TempDir()

	out, err := NewProject(root).Execute(context.Background(), ecosystem.Environment{ID: root},
		ecosystem.Batch{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "foo/bar", Version: "^1.2"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("expected the raw composer output to be returned")
	}
	invs := readComposerInvocations(t, logFile)
	if len(invs) != 1 {
		t.Fatalf("got %d invocations, want 1", len(invs))
	}
	assertExactArgs(t, invs[0], root, []string{"require", "foo/bar:^1.2", "--no-interaction"})
}

func TestExecuteRemoveInvokesComposerRemove(t *testing.T) {
	logFile := fakeComposerShim(t)
	root := t.TempDir()

	if _, err := NewProject(root).Execute(context.Background(), ecosystem.Environment{ID: root},
		ecosystem.Batch{Op: ecosystem.OpRemove, Items: []ecosystem.Item{{Name: "foo/bar"}}}); err != nil {
		t.Fatal(err)
	}
	invs := readComposerInvocations(t, logFile)
	if len(invs) != 1 {
		t.Fatalf("got %d invocations, want 1", len(invs))
	}
	assertExactArgs(t, invs[0], root, []string{"remove", "foo/bar", "--no-interaction"})
}

func TestExecuteBatchBatchesItemsPerOpKind(t *testing.T) {
	logFile := fakeComposerShim(t)
	root := t.TempDir()

	if _, err := NewProject(root).Execute(context.Background(), ecosystem.Environment{ID: root},
		ecosystem.Batch{Op: ecosystem.OpUpgrade, Items: []ecosystem.Item{{Name: "a/b", Version: "2.0.1"}, {Name: "c/d"}}}); err != nil {
		t.Fatal(err)
	}
	invs := readComposerInvocations(t, logFile)
	if len(invs) != 1 {
		t.Fatalf("got %d invocations, want one batched invocation per op kind", len(invs))
	}
	assertExactArgs(t, invs[0], root, []string{"require", "a/b:2.0.1", "c/d", "--no-interaction"})
}

func TestExecuteMissingComposerToolchain(t *testing.T) {
	t.Setenv("PATH", "")
	root := t.TempDir()

	_, err := NewProject(root).Execute(context.Background(), ecosystem.Environment{ID: root},
		ecosystem.Batch{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "x"}}})
	if err == nil {
		t.Fatal("expected a load failure when no composer is on PATH")
	}
	msg := err.Error()
	if !strings.Contains(msg, "composer") || !strings.Contains(strings.ToLower(msg), "not found on path") {
		t.Fatalf("error must name the missing binary and state it is not on PATH: %q", msg)
	}
}
