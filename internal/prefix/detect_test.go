package prefix

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// buildFixture creates a fake home with nvm (3 versions, one lacking node),
// fnm (1 version), volta (1 version) layouts. Each stub node prints its
// version when executed.
func buildFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()

	mkPrefix := func(p, version string, pkgs ...string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(p, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeStub(t, filepath.Join(p, "bin", "node"), version)
		if err := os.MkdirAll(filepath.Join(p, "lib", "node_modules", "npm"), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, pkg := range pkgs {
			d := filepath.Join(p, "lib", "node_modules", pkg)
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(d, "package.json"), []byte(`{}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	nvm := filepath.Join(home, ".nvm", "versions", "node")
	mkPrefix(filepath.Join(nvm, "v24.12.0"), "v24.12.0", "alpha", "beta")
	mkPrefix(filepath.Join(nvm, "v22.15.0"), "v22.15.0", "gamma")
	mkPrefix(filepath.Join(nvm, "v14.21.3"), "v14.21.3")
	// a version dir without bin/node must be skipped
	if err := os.MkdirAll(filepath.Join(nvm, "v99.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}

	fnm := filepath.Join(home, ".local", "share", "fnm", "node-versions")
	mkPrefix(filepath.Join(fnm, "v20.11.1", "installation"), "v20.11.1", "delta")

	volta := filepath.Join(home, ".volta", "tools", "image", "node")
	mkPrefix(filepath.Join(volta, "20.3.0"), "20.3.0", "epsilon", "@scope/fed")

	return home
}

func TestDetectFixtureLayouts(t *testing.T) {
	home := buildFixture(t)
	cfg := Config{
		HomeDir:  home,
		LookEnv:  noEnv,
		Path:     filepath.Join(home, "none"),
		ActiveFn: func(context.Context) (string, error) { return "", os.ErrNotExist },
	}

	infos, err := Detect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]Info{}
	for _, i := range infos {
		byID[i.ID] = i
	}

	wantNvm24 := filepath.Join(home, ".nvm", "versions", "node", "v24.12.0")
	if i, ok := byID[wantNvm24]; !ok || i.Source != "nvm" || i.NodeVersion != "v24.12.0" || i.PkgCount != 3 {
		t.Fatalf("nvm v24 = %+v (want source nvm, version v24.12.0, 3 pkgs incl npm)", i)
	}
	wantNvm22 := filepath.Join(home, ".nvm", "versions", "node", "v22.15.0")
	if i, ok := byID[wantNvm22]; !ok || i.PkgCount != 2 {
		t.Fatalf("nvm v22 = %+v (want 2 pkgs incl npm)", i)
	}
	wantNvm14 := filepath.Join(home, ".nvm", "versions", "node", "v14.21.3")
	if i, ok := byID[wantNvm14]; !ok || i.PkgCount != 1 {
		t.Fatalf("nvm v14 = %+v (want only npm)", i)
	}
	if _, ok := byID[filepath.Join(home, ".nvm", "versions", "node", "v99.0.0")]; ok {
		t.Fatal("version dir without bin/node must not be listed")
	}

	wantFnm := filepath.Join(home, ".local", "share", "fnm", "node-versions", "v20.11.1", "installation")
	if i, ok := byID[wantFnm]; !ok || i.Source != "fnm" || i.NodeVersion != "v20.11.1" || i.PkgCount != 2 {
		t.Fatalf("fnm = %+v (want source fnm, v20.11.1, 2 pkgs)", i)
	}

	wantVolta := filepath.Join(home, ".volta", "tools", "image", "node", "20.3.0")
	if i, ok := byID[wantVolta]; !ok || i.Source != "volta" || i.NodeVersion != "v20.3.0" || i.PkgCount != 3 {
		t.Fatalf("volta = %+v (want source volta, v20.3.0, 3 pkgs incl scoped)", i)
	}

	if len(infos) != 5 {
		t.Fatalf("detected %d prefixes, want 5 (no active npm in fixture): %v", len(infos), ids(infos))
	}

	// sorted by version descending: v24 first
	if infos[0].NodeVersion != "v24.12.0" {
		t.Fatalf("first entry = %s, want v24.12.0 (version desc)", infos[0].NodeVersion)
	}
}

func TestDetectEnvOverrides(t *testing.T) {
	home := t.TempDir()
	altNvm := t.TempDir()
	p := filepath.Join(altNvm, "versions", "node", "v18.0.0")
	if err := os.MkdirAll(filepath.Join(p, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "bin", "node"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(p, "lib", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}

	envs := map[string]string{"NVM_DIR": altNvm}
	cfg := Config{HomeDir: home, Path: filepath.Join(home, "none"), LookEnv: func(k string) (string, bool) {
		v, ok := envs[k]
		return v, ok
	}}
	infos, err := Detect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, i := range infos {
		if i.ID == p && i.Source == "nvm" {
			found = true
		}
	}
	if !found {
		t.Fatalf("NVM_DIR override not honored: %v", ids(infos))
	}
}

func TestDetectSystemOnly(t *testing.T) {
	home := t.TempDir() // empty home: no nvm/fnm/volta
	cfg := Config{HomeDir: home, LookEnv: noEnv, Path: filepath.Join(home, "none")}
	infos, err := Detect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	// The machine's active npm prefix should still appear (or none if no npm).
	for _, i := range infos {
		if i.Source != "system" || !i.Active {
			t.Fatalf("unexpected non-system entry in system-only fixture: %+v", i)
		}
	}
}

func TestPathCandidates(t *testing.T) {
	root := t.TempDir()
	binA := filepath.Join(root, "a", "bin")
	binB := filepath.Join(root, "b", "bin")
	empty := filepath.Join(root, "empty")
	for _, d := range []string{binA, binB, empty} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeStub(t, filepath.Join(binA, "node"), "v18.0.0")
	writeStub(t, filepath.Join(binB, "nodejs"), "v20.5.0")

	hits := pathCandidates(binA + ":" + binB + ":" + empty + ":")
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2: %+v", len(hits), hits)
	}
	if hits[0].prefix != filepath.Join(root, "a") || hits[0].bin != filepath.Join(binA, "node") {
		t.Fatalf("hit 0 = %+v (want prefix %s, bin %s)", hits[0], filepath.Join(root, "a"), filepath.Join(binA, "node"))
	}
	if hits[1].prefix != filepath.Join(root, "b") || hits[1].bin != filepath.Join(binB, "nodejs") {
		t.Fatalf("hit 1 = %+v (want prefix %s, bin %s)", hits[1], filepath.Join(root, "b"), filepath.Join(binB, "nodejs"))
	}
}

func TestDetectSystemNodeViaPath(t *testing.T) {
	home := t.TempDir()
	us := filepath.Join(home, "usr")
	writeStub(t, filepath.Join(us, "bin", "node"), "v18.0.0")
	opt := filepath.Join(home, "opt")
	writeStub(t, filepath.Join(opt, "bin", "nodejs"), "v20.5.0")
	broken := filepath.Join(home, "broken", "bin")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "node"), []byte("not a real node\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		HomeDir:  home,
		LookEnv:  noEnv,
		Path:     filepath.Join(us, "bin") + ":" + filepath.Join(opt, "bin") + ":" + broken,
		ActiveFn: noActive,
	}
	infos, err := Detect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatalf("detected %d prefixes, want 2 (non-executable dropped): %v", len(infos), ids(infos))
	}
	if infos[0].ID != opt || infos[0].Source != "system" || infos[0].NodeVersion != "v20.5.0" || infos[0].Active {
		t.Fatalf("first = %+v (want system v20.5.0 from nodejs, inactive)", infos[0])
	}
	if infos[1].ID != us || infos[1].Source != "system" || infos[1].NodeVersion != "v18.0.0" {
		t.Fatalf("second = %+v (want system v18.0.0 from node)", infos[1])
	}
}

func TestDetectPathIntoNvmNotDuplicated(t *testing.T) {
	home := buildFixture(t)
	nvm24 := filepath.Join(home, ".nvm", "versions", "node", "v24.12.0")
	us := filepath.Join(home, "usr")
	writeStub(t, filepath.Join(us, "bin", "node"), "v18.0.0")

	cfg := Config{
		HomeDir:  home,
		LookEnv:  noEnv,
		Path:     filepath.Join(nvm24, "bin") + ":" + filepath.Join(us, "bin"),
		ActiveFn: noActive,
	}
	infos, err := Detect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 6 {
		t.Fatalf("detected %d prefixes, want 6 (5 fixture + 1 system): %v", len(infos), ids(infos))
	}
	n := 0
	for _, i := range infos {
		if i.ID == nvm24 {
			n++
			if i.Source != "nvm" || i.Active {
				t.Fatalf("nvm v24 entry altered by PATH scan: %+v", i)
			}
		}
	}
	if n != 1 {
		t.Fatalf("nvm v24 prefix appears %d times, want exactly 1", n)
	}
	var sys *Info
	for i := range infos {
		if infos[i].ID == us {
			sys = &infos[i]
		}
	}
	if sys == nil || sys.Source != "system" || sys.NodeVersion != "v18.0.0" {
		t.Fatalf("system node missing from results: %+v", sys)
	}
}

func TestDetectPathSymlinkDedup(t *testing.T) {
	home := t.TempDir()
	real := filepath.Join(home, "real")
	writeStub(t, filepath.Join(real, "bin", "node"), "v16.0.0")
	prefixAlias := filepath.Join(home, "alias") // symlink to the prefix
	if err := os.Symlink(real, prefixAlias); err != nil {
		t.Fatal(err)
	}
	binAlias := filepath.Join(home, "binlink") // symlink to the bin dir (merged-usr style)
	if err := os.Symlink(filepath.Join(real, "bin"), binAlias); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		HomeDir:  home,
		LookEnv:  noEnv,
		Path:     filepath.Join(real, "bin") + ":" + filepath.Join(prefixAlias, "bin") + ":" + binAlias,
		ActiveFn: noActive,
	}
	infos, err := Detect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("detected %d prefixes, want 1 (aliases deduped): %v", len(infos), ids(infos))
	}
	if infos[0].ID != real || infos[0].Source != "system" || infos[0].NodeVersion != "v16.0.0" {
		t.Fatalf("entry = %+v (want %s system v16.0.0)", infos[0], real)
	}
}

func TestDetectActiveUnchangedWithSystemOnPath(t *testing.T) {
	home := buildFixture(t)
	nvm24 := filepath.Join(home, ".nvm", "versions", "node", "v24.12.0")
	us := filepath.Join(home, "usr")
	writeStub(t, filepath.Join(us, "bin", "node"), "v30.0.0") // newer than any nvm version

	cfg := Config{
		HomeDir:  home,
		LookEnv:  noEnv,
		Path:     filepath.Join(us, "bin"),
		ActiveFn: func(context.Context) (string, error) { return nvm24, nil },
	}
	infos, err := Detect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, i := range infos {
		if i.Active {
			active++
			if i.ID != nvm24 || i.Source != "nvm" {
				t.Fatalf("active env = %+v, want the nvm v24 prefix", i)
			}
		}
	}
	if active != 1 {
		t.Fatalf("%d active envs, want 1: %v", active, ids(infos))
	}
	var sys *Info
	for i := range infos {
		if infos[i].ID == us {
			sys = &infos[i]
		}
	}
	if sys == nil || sys.Source != "system" || sys.Active {
		t.Fatalf("system node on PATH must be listed but inactive: %+v", sys)
	}
}

func TestDetectActiveMarkedWhenDiscoveredViaPath(t *testing.T) {
	home := t.TempDir()
	us := filepath.Join(home, "usr")
	writeStub(t, filepath.Join(us, "bin", "node"), "v18.0.0")

	cfg := Config{
		HomeDir:  home,
		LookEnv:  noEnv,
		Path:     filepath.Join(us, "bin"),
		ActiveFn: func(context.Context) (string, error) { return us, nil },
	}
	infos, err := Detect(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].ID != us || infos[0].Source != "system" || !infos[0].Active {
		t.Fatalf("infos = %+v, want single active system env %s", infos, us)
	}
}

func ids(infos []Info) []string {
	out := make([]string, len(infos))
	for i, x := range infos {
		out[i] = filepath.Base(x.ID) + "(" + x.Source + ")"
	}
	return out
}

// writeStub writes an executable shell script that prints version.
func writeStub(t *testing.T, path, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho "+version+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func noEnv(string) (string, bool) { return "", false }

func noActive(context.Context) (string, error) { return "", os.ErrNotExist }
