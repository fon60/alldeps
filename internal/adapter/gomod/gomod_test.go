package gomod

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"npmitude/internal/ecosystem"
)

func TestDetectProjectGoMod(t *testing.T) {
	root := t.TempDir()
	if ok, meta := New().DetectProject(root); ok {
		t.Fatalf("DetectProject on an empty dir = true (meta %v), want not applicable", meta)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/fixture\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, meta := New().DetectProject(root)
	if !ok {
		t.Fatal("DetectProject with a go.mod present = false, want applicable")
	}
	if len(meta) != 0 {
		t.Fatalf("meta = %v, want none (no variant for Go)", meta)
	}

	dirAsGoMod := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dirAsGoMod, "go.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if ok, _ := New().DetectProject(dirAsGoMod); ok {
		t.Fatal("a go.mod directory must not count as a Go module project")
	}
}

func TestCapabilitiesProjectScopeOnly(t *testing.T) {
	caps := New().Capabilities()
	if !caps.ProjectScope {
		t.Fatal("gomod adapter must advertise ProjectScope")
	}
	for name, v := range map[string]bool{
		"GlobalScope":           caps.GlobalScope,
		"HasSearch":             caps.HasSearch,
		"HasDedupe":             caps.HasDedupe,
		"HasNativeLock":         caps.HasNativeLock,
		"HasConflictResolution": caps.HasConflictResolution,
	} {
		if v {
			t.Fatalf("capability %s must be false for Go", name)
		}
	}
}

func TestDiscoverReturnsProjectRoot(t *testing.T) {
	root := t.TempDir()
	envs, err := NewProject(root).Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Fatalf("got %d environments, want 1: %+v", len(envs), envs)
	}
	e := envs[0]
	if e.ID != root {
		t.Fatalf("env ID = %q, want the project root", e.ID)
	}
	if e.Kind != envKind {
		t.Fatalf("env kind = %q, want %q", e.Kind, envKind)
	}
	if e.Meta[ecosystem.MetaSource] != "project" {
		t.Fatalf("meta source = %q, want project", e.Meta[ecosystem.MetaSource])
	}
}

// fakeGoShim puts a `go` script first on PATH that records every invocation
// (cwd plus one arg per line) to a log file and answers `list` by catting
// fixture. It returns the log file path.
func fakeGoShim(t *testing.T, fixture string) string {
	t.Helper()
	bindir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "go.log")
	script := fmt.Sprintf(`#!/bin/sh
{
  printf 'cwd=%%s\n' "$PWD"
  for a in "$@"; do printf 'arg=%%s\n' "$a"; done
} >> %q
if [ "$1" = "list" ]; then cat %q; exit 0; fi
exit 0
`, logFile, fixture)
	if err := os.WriteFile(filepath.Join(bindir, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logFile
}

type goInvocation struct {
	cwd  string
	args []string
}

// readGoInvocations parses the shim log into per-invocation cwd + argv.
func readGoInvocations(t *testing.T, logFile string) []goInvocation {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("no go invocations recorded: %v", err)
	}
	var invs []goInvocation
	var cur *goInvocation
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "cwd=") {
			invs = append(invs, goInvocation{cwd: strings.TrimPrefix(line, "cwd=")})
			cur = &invs[len(invs)-1]
			continue
		}
		if strings.HasPrefix(line, "arg=") && cur != nil {
			cur.args = append(cur.args, strings.TrimPrefix(line, "arg="))
		}
	}
	return invs
}
