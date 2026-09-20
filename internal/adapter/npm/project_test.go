package npm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"npmitude/internal/ecosystem"
)

func writeProject(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(root, f), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectProjectVariants(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  string // "" = not applicable
	}{
		{"pnpm lockfile", []string{"package.json", "pnpm-lock.yaml"}, "pnpm"},
		{"yarn lockfile", []string{"package.json", "yarn.lock"}, "yarn"},
		{"npm lockfile", []string{"package.json", "package-lock.json"}, "npm"},
		{"bare package.json defaults to npm", []string{"package.json"}, "npm"},
		{"no markers", nil, ""},
		{"other files only", []string{"composer.json", "go.mod"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeProject(t, root, tc.files...)
			ok, meta := New().DetectProject(root)
			if tc.want == "" {
				if ok {
					t.Fatalf("DetectProject = true, want not applicable (meta %v)", meta)
				}
				return
			}
			if !ok {
				t.Fatal("DetectProject = false, want applicable")
			}
			if got := meta[ecosystem.MetaVariant]; got != tc.want {
				t.Fatalf("variant = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCapabilitiesAdvertiseProjectScope(t *testing.T) {
	caps := New().Capabilities()
	if !caps.ProjectScope {
		t.Fatal("npm adapter must advertise ProjectScope")
	}
	if !caps.GlobalScope {
		t.Fatal("npm adapter must keep GlobalScope")
	}
}

func TestProjectDiscoverReturnsModuleDir(t *testing.T) {
	root := t.TempDir()
	envs, err := NewProject(root).Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Fatalf("got %d environments, want 1: %+v", len(envs), envs)
	}
	e := envs[0]
	if e.ID != filepath.Join(root, "node_modules") {
		t.Fatalf("env ID = %q, want the project's node_modules dir", e.ID)
	}
	if e.Kind != projectEnvKind {
		t.Fatalf("env kind = %q, want %q", e.Kind, projectEnvKind)
	}
	if e.Meta[ecosystem.MetaSource] != "project" {
		t.Fatalf("meta source = %q, want project", e.Meta[ecosystem.MetaSource])
	}
}

// fakeActivePrefix puts a fake npm on PATH answering `config get prefix` with dir.
func fakeActivePrefix(t *testing.T, dir string) {
	t.Helper()
	bindir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"config\" ]; then printf '%%s\\n' %q; exit 0; fi\nexit 1\n", dir)
	if err := os.WriteFile(filepath.Join(bindir, "npm"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestProjectListInstalledUsesLocalLS(t *testing.T) {
	prefixDir := fakePrefix(t, recordedLS, "https://registry.example/")
	fakeActivePrefix(t, prefixDir)
	root := t.TempDir()
	writeProject(t, root, "package.json")

	e := NewProject(root)
	envs, err := e.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := e.ListInstalled(context.Background(), envs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 3 {
		t.Fatalf("got %d packages, want 3: %+v", len(pkgs), pkgs)
	}
}

func TestProjectExecuteIsScopedToRootWithoutGlobalFlag(t *testing.T) {
	prefixDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "calls.log")
	bin := filepath.Join(prefixDir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("#!/bin/sh\necho \"cwd=$PWD args=$*\" >> %q\nexit 0\n", logFile)
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeActivePrefix(t, prefixDir)

	root := t.TempDir()
	e := NewProject(root)
	_, err := e.Execute(context.Background(), ecosystem.Environment{ID: filepath.Join(root, "node_modules")},
		ecosystem.Batch{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "foo", Version: "1.2.3"}}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	call := string(data)
	if strings.Contains(call, "-g") {
		t.Fatalf("project execution must not use the global flag: %s", call)
	}
	if !strings.Contains(call, "cwd="+root) {
		t.Fatalf("project execution must be rooted in the project dir: %s", call)
	}
	if !strings.Contains(call, "i foo@1.2.3") {
		t.Fatalf("install batch not passed through: %s", call)
	}
}

func TestProjectResolveLabelsOmitGlobalFlag(t *testing.T) {
	e := NewProject(t.TempDir())
	plan, _, err := e.Resolve(ecosystem.Intent{
		Env:   ecosystem.Environment{ID: "/p/node_modules"},
		Items: []ecosystem.MarkedItem{{Op: ecosystem.OpInstall, Name: "foo", Version: "1.2.3"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Batches) != 1 || plan.Batches[0].Label != "npm i foo@1.2.3" {
		t.Fatalf("batches = %+v, want a single 'npm i foo@1.2.3'", plan.Batches)
	}
}
