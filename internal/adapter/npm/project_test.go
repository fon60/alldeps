package npm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

// hermeticProjectEnv points all version-manager discovery at empty dirs and
// restricts PATH to the given dirs plus a minimal utility dir (sh, cat), so
// NewProject's construction-time detection never touches real machine state
// while fixture shell scripts keep working.
func hermeticProjectEnv(t *testing.T, pathDirs ...string) {
	t.Helper()
	none := filepath.Join(t.TempDir(), "absent")
	t.Setenv("NVM_DIR", none)
	t.Setenv("FNM_DIR", none)
	t.Setenv("VOLTA_HOME", none)
	utils := t.TempDir()
	for _, tool := range []string{"sh", "cat"} {
		src, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("cannot locate %s for the hermetic PATH: %v", tool, err)
		}
		if err := os.Symlink(src, filepath.Join(utils, tool)); err != nil {
			t.Fatal(err)
		}
	}
	parts := append([]string{}, pathDirs...)
	parts = append(parts, utils)
	t.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))
}

func TestProjectDiscoverReturnsModuleDir(t *testing.T) {
	root := t.TempDir()
	hermeticProjectEnv(t)
	envs, err := NewProject(context.Background(), root).Discover(context.Background())
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
	hermeticProjectEnv(t)
	fakeActivePrefix(t, prefixDir)
	root := t.TempDir()
	writeProject(t, root, "package.json")

	e := NewProject(context.Background(), root)
	envs, err := e.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := e.ListInstalled(context.Background(), envs[0])
	if err != nil {
		t.Fatal(err)
	}
	// The full tree is listed: the three top-level packages plus the nested
	// peer-x as automatic.
	if len(pkgs) != 4 {
		t.Fatalf("got %d packages, want 4 (nested dependency included): %+v", len(pkgs), pkgs)
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
	npmCLI := filepath.Join(prefixDir, "lib", "node_modules", "npm", "bin", "npm-cli.js")
	if err := os.MkdirAll(filepath.Dir(npmCLI), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(npmCLI, []byte("# fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hermeticProjectEnv(t)
	fakeActivePrefix(t, prefixDir)

	root := t.TempDir()
	e := NewProject(context.Background(), root)
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
	hermeticProjectEnv(t)
	e := NewProject(context.Background(), t.TempDir())
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

// fakeNVMVersion adds one nvm-layout prefix (an executable bin/node with the
// given script body) under base and returns its path.
func fakeNVMVersion(t *testing.T, base, version, nodeScript string) string {
	t.Helper()
	p := filepath.Join(base, "versions", "node", version)
	if err := os.MkdirAll(filepath.Join(p, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "bin", "node"), []byte(nodeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// loggingPrefixAt builds a full standard-layout prefix at dir whose bin/node
// logs every invocation (its own path + args) to logFile before answering npm
// invocations: -p -> version, ls -> lsJSON, config get registry -> registryURL.
func loggingPrefixAt(t *testing.T, dir, version, lsJSON, registryURL, logFile string) string {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "ls.json")
	if err := os.WriteFile(fixture, []byte(lsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s %%s\n' "$0" "$*" >> %q
if [ "$1" = "-p" ]; then printf '%%s\n' %q; exit 0; fi
shift
case "$1" in
  ls) cat %q; exit 0;;
  config) printf '%%s\n' %q; exit 0;;
esac
exit 1
`, logFile, version, fixture, registryURL)
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	npmCLI := filepath.Join(dir, "lib", "node_modules", "npm", "bin", "npm-cli.js")
	if err := os.MkdirAll(filepath.Dir(npmCLI), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(npmCLI, []byte("# fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestProjectListRunsOnPinnedPrefix(t *testing.T) {
	base := t.TempDir()
	logs := map[string]string{}
	for _, version := range []string{"v20.19.1", "v22.15.0"} {
		logs[version] = filepath.Join(t.TempDir(), version+".log")
		loggingPrefixAt(t, filepath.Join(base, "versions", "node", version), version, recordedLS, "https://registry.example/", logs[version])
	}
	activeLog := filepath.Join(t.TempDir(), "active.log")
	activeDir := loggingPrefixAt(t, t.TempDir(), "v18.0.0", recordedLS, "https://registry.example/", activeLog)

	hermeticProjectEnv(t)
	t.Setenv("NVM_DIR", base)
	fakeActivePrefix(t, activeDir)

	root := t.TempDir()
	writeProject(t, root, "package.json")
	if err := os.WriteFile(filepath.Join(root, ".nvmrc"), []byte("22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := NewProject(context.Background(), root)
	envs, err := e.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ListInstalled(context.Background(), envs[0]); err != nil {
		t.Fatal(err)
	}

	pinnedLog, err := os.ReadFile(logs["v22.15.0"])
	if err != nil {
		t.Fatalf("the pinned prefix's node was never invoked: %v", err)
	}
	if !strings.Contains(string(pinnedLog), "ls --all") {
		t.Fatalf("pinned node did not run the project ls: %s", pinnedLog)
	}
	for name, f := range map[string]string{"active": activeLog, "v20.19.1": logs["v20.19.1"]} {
		if data, err := os.ReadFile(f); err == nil && strings.Contains(string(data), "ls --all") {
			t.Fatalf("the %s node ran the project ls instead of the pinned one: %s", name, data)
		}
	}
}

func TestNewProjectBindsPinnedPrefix(t *testing.T) {
	base := t.TempDir()
	p20 := fakeNVMVersion(t, base, "v20.19.1", "#!/bin/sh\nexit 0\n")
	p22a := fakeNVMVersion(t, base, "v22.11.0", "#!/bin/sh\nexit 0\n")
	p22b := fakeNVMVersion(t, base, "v22.15.0", "#!/bin/sh\nexit 0\n")
	activeDir := t.TempDir() // bare dir: no standard layout, the PATH npm answers for it
	hermeticProjectEnv(t)
	t.Setenv("NVM_DIR", base)
	fakeActivePrefix(t, activeDir)

	cases := []struct {
		name   string
		nvmrc  string // "" = no .nvmrc file
		want   string
		notice bool
	}{
		{"full pin binds exactly", "v20.19.1", p20, false},
		{"partial major binds highest", "22", p22b, false},
		{"partial minor binds highest", "22.11", p22a, false},
		{"uninstalled pin falls back with notice", "24", activeDir, true},
		{"absent pin falls back silently", "", activeDir, false},
		{"unparseable pin falls back silently", "lts/*", activeDir, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.nvmrc != "" {
				if err := os.WriteFile(filepath.Join(root, ".nvmrc"), []byte(tc.nvmrc+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			e := NewProject(context.Background(), root)
			if e.projectPrefix != tc.want {
				t.Fatalf("bound prefix = %q, want %q", e.projectPrefix, tc.want)
			}
			got := e.PinNotice()
			if (got != "") != tc.notice {
				t.Fatalf("PinNotice = %q, want notice present = %v", got, tc.notice)
			}
			if tc.notice && !strings.Contains(got, "24") {
				t.Fatalf("notice must name the pinned version: %q", got)
			}
		})
	}
}
