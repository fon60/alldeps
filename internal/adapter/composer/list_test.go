package composer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"npmitude/internal/ecosystem"
)

const installedFixture = `{
    "packages": [
        {
            "name": "psr/log",
            "version": "3.0.2",
            "require": {"php": ">=8.0.0"}
        },
        {
            "name": "vendor/alpha",
            "version": "1.2.3"
        },
        {
            "name": "vendor/transitive",
            "version": "2.0.0"
        }
    ],
    "dev": false,
    "dev-package-names": []
}`

func writeProject(t *testing.T, composerJSON string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(composerJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeInstalled(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, "vendor", "composer")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "installed.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListInstalledFullSetWithExactVersions(t *testing.T) {
	root := writeProject(t, `{"require":{"psr/log":"^3.0","vendor/alpha":"~1.2","php":"^8.1"}}`)
	writeInstalled(t, root, installedFixture)

	pkgs, err := NewProject(root).ListInstalled(context.Background(), ecosystem.Environment{ID: root})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ecosystem.Package{}
	for _, p := range pkgs {
		got[p.Name] = p
	}
	want := map[string]struct {
		version   string
		automatic bool
	}{
		"psr/log":          {"3.0.2", false},
		"vendor/alpha":     {"1.2.3", false},
		"vendor/transitive": {"2.0.0", true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d packages %v, want exactly %d (transitive listed, platform reqs absent)", len(got), got, len(want))
	}
	for name, w := range want {
		p, ok := got[name]
		if !ok {
			t.Fatalf("%s missing from the list", name)
		}
		if p.Version != w.version {
			t.Fatalf("%s version = %q, want the exact installed %q (not the declared constraint)", name, p.Version, w.version)
		}
		if p.Automatic != w.automatic {
			t.Fatalf("%s automatic = %v, want %v", name, p.Automatic, w.automatic)
		}
	}
	if _, ok := got["php"]; ok {
		t.Fatal("platform requirement php must not be listed as a row")
	}
}

func TestListInstalledWithoutVendorIsEmpty(t *testing.T) {
	root := writeProject(t, `{"require":{"psr/log":"^3.0"}}`)

	pkgs, err := NewProject(root).ListInstalled(context.Background(), ecosystem.Environment{ID: root})
	if err != nil {
		t.Fatalf("no vendor dir must not be a load failure: %v", err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("got %d packages without vendor, want an empty list", len(pkgs))
	}
}

func TestListInstalledUnparseableMetadataIsLoadFailure(t *testing.T) {
	root := writeProject(t, `{"require":{"psr/log":"^3.0"}}`)
	writeInstalled(t, root, "not json at all\n")

	if _, err := NewProject(root).ListInstalled(context.Background(), ecosystem.Environment{ID: root}); err == nil {
		t.Fatal("expected a load failure on unparseable installed.json")
	}
}

func TestListInstalledUnparseableComposerJSONIsLoadFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewProject(root).ListInstalled(context.Background(), ecosystem.Environment{ID: root}); err == nil {
		t.Fatal("expected a load failure on unparseable composer.json")
	}
}
