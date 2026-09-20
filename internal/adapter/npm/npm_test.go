package npm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"npmitude/internal/ecosystem"
	"npmitude/internal/lock"
)

// recordedLS is a fixture of `npm ls -g --all --json` output.
const recordedLS = `{
  "name": null,
  "dependencies": {
    "pad-left": {"version": "2.3.0"},
    "left-pad": {"version": "1.3.0", "missing": true},
    "deep": {"version": "1.0.0", "dependencies": {"peer-x": {"version": "0.0.1", "invalid": true}}}
  }
}`

// fakePrefix builds a throwaway prefix whose bin/node is a shell script that
// answers npm's invocations from recorded fixtures: ls -> lsJSON, config get
// registry -> registryURL, -p -> nodeVersion.
func fakePrefix(t *testing.T, lsJSON, registryURL string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "ls.json")
	if err := os.WriteFile(fixture, []byte(lsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "-p" ]; then printf '%%s\n' "v18.0.0"; exit 0; fi
shift
case "$1" in
  ls) cat %q; exit 0;;
  config) printf '%%s\n' %q; exit 0;;
esac
exit 1
`, fixture, registryURL)
	node := filepath.Join(bin, "node")
	if err := os.WriteFile(node, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestListInstalledAgainstRecordedLS(t *testing.T) {
	env := ecosystem.Environment{ID: fakePrefix(t, recordedLS, "https://registry.example/")}
	pkgs, err := New().ListInstalled(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]ecosystem.Package{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	if len(pkgs) != 3 {
		t.Fatalf("got %d packages, want 3: %+v", len(pkgs), pkgs)
	}
	if p := byName["pad-left"]; p.Version != "2.3.0" || p.Unhealthy {
		t.Fatalf("pad-left = %+v, want 2.3.0 healthy", p)
	}
	if p := byName["left-pad"]; !p.Unhealthy {
		t.Fatalf("left-pad = %+v, want unhealthy (missing)", p)
	}
	if p := byName["deep"]; !p.Unhealthy {
		t.Fatalf("deep = %+v, want unhealthy (invalid nested dep)", p)
	}
}

func TestSearchAgainstRegistryStub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/-/v1/search") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `{"objects":[{"package":{"name":"pad-left","version":"2.3.0","description":"Pads a string with zeros (left)"}}],"total":1}`)
	}))
	defer srv.Close()

	env := ecosystem.Environment{ID: fakePrefix(t, recordedLS, srv.URL)}
	hits, total, err := New().Search(context.Background(), env, "pad", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(hits) != 1 {
		t.Fatalf("hits = %+v total = %d, want 1 hit / total 1", hits, total)
	}
	if hits[0].Name != "pad-left" || hits[0].Version != "2.3.0" || hits[0].Description == "" {
		t.Fatalf("hit = %+v", hits[0])
	}
}

func TestDiscoverAgainstNVMLayout(t *testing.T) {
	base := t.TempDir()
	prefixID := filepath.Join(base, "versions", "node", "v18.0.0")
	if err := os.MkdirAll(filepath.Join(prefixID, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	nodeScript := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(prefixID, "bin", "node"), []byte(nodeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(prefixID, "lib", "node_modules", "pad-left"), 0o755); err != nil {
		t.Fatal(err)
	}

	fakebin := t.TempDir()
	npmScript := "#!/bin/sh\n" + fmt.Sprintf("printf '%s\\n'", prefixID) + "\n"
	if err := os.WriteFile(filepath.Join(fakebin, "npm"), []byte(npmScript), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("NVM_DIR", base)
	t.Setenv("FNM_DIR", filepath.Join(base, "no-fnm"))
	t.Setenv("VOLTA_HOME", filepath.Join(base, "no-volta"))
	t.Setenv("PATH", fakebin+string(os.PathListSeparator)+os.Getenv("PATH"))

	envs, err := New().Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Fatalf("got %d environments, want 1: %+v", len(envs), envs)
	}
	e := envs[0]
	if e.ID != prefixID || e.Kind != "node-prefix" || e.Rank != "v18.0.0" {
		t.Fatalf("env = %+v", e)
	}
	if e.Meta[ecosystem.MetaSource] != "nvm" || e.Meta[ecosystem.MetaPkgCount] != "1" || e.Meta[ecosystem.MetaActive] != "1" {
		t.Fatalf("meta = %v, want nvm source / 1 pkg / active", e.Meta)
	}
}

func TestResolveBatchesByOpKindWithNoConflicts(t *testing.T) {
	intent := ecosystem.Intent{
		Env: ecosystem.Environment{ID: "/p"},
		Items: []ecosystem.MarkedItem{
			{Op: ecosystem.OpInstall, Name: "foo", Version: "1.2.3"},
			{Op: ecosystem.OpRemove, Name: "old"},
			{Op: ecosystem.OpUpgrade, Name: "bar", Version: "2.0.0"},
			{Op: ecosystem.OpInstall, Name: "baz"},
		},
	}
	eco := New()
	plan, conflicts, err := eco.Resolve(intent)
	if err != nil {
		t.Fatal(err)
	}
	// The npm resolver is inert with respect to conflicts (the feature is
	// dormant for npm except the dedupe flavor), while dedupe itself is
	// advertised so the app can derive redundancy conflicts.
	if len(conflicts) != 0 {
		t.Fatalf("npm resolver must not produce conflicts, got %+v", conflicts)
	}
	caps := eco.Capabilities()
	if !caps.HasDedupe {
		t.Fatal("npm must advertise HasDedupe (the Node family carries redundant copies)")
	}
	if caps.HasConflictResolution {
		t.Fatal("npm must not claim resolver-produced conflict resolution")
	}
	if len(plan.Batches) != 3 {
		t.Fatalf("got %d batches, want 3 (install, upgrade, remove): %+v", len(plan.Batches), plan.Batches)
	}
	want := []string{"npm i -g foo@1.2.3 baz", "npm i -g bar@2.0.0", "npm rm -g old"}
	for i, b := range plan.Batches {
		if b.Label != want[i] {
			t.Fatalf("batch %d label = %q, want %q", i, b.Label, want[i])
		}
	}
}

func TestLockFallsBackToSessionLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	eco := New()
	if eco.Capabilities().HasNativeLock {
		t.Fatal("npm must not claim a native lock")
	}
	h, err := eco.Lock(ecosystem.Environment{ID: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	lm := lock.New(filepath.Join(dir, "npmitude", "locks"))
	if !lm.IsHeld("/p") {
		t.Fatal("Lock must take the session-lock fallback path for npm")
	}
	h.Release()
	if lm.IsHeld("/p") {
		t.Fatal("Release must drop the session lock")
	}
}
