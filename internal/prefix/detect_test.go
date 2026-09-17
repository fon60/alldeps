package prefix

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// buildFixture creates a fake home with nvm (3 versions, one lacking node),
// fnm (1 version), volta (1 version) layouts.
func buildFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()

	mkPrefix := func(p, version string, pkgs ...string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(p, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "bin", "node"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
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
		_ = version
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
		LookEnv:  func(string) (string, bool) { return "", false },
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
	cfg := Config{HomeDir: home, LookEnv: func(k string) (string, bool) {
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
	cfg := Config{HomeDir: home, LookEnv: func(string) (string, bool) { return "", false }}
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

func ids(infos []Info) []string {
	out := make([]string, len(infos))
	for i, x := range infos {
		out[i] = filepath.Base(x.ID) + "(" + x.Source + ")"
	}
	return out
}
