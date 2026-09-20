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

func TestListInstalledDirectDepsWithExactVersions(t *testing.T) {
	root := writeProject(t, `{"require":{"psr/log":"^3.0","vendor/alpha":"~1.2","php":"^8.1"}}`)
	writeInstalled(t, root, installedFixture)

	pkgs, err := NewProject(root).ListInstalled(context.Background(), ecosystem.Environment{ID: root})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range pkgs {
		got[p.Name] = p.Version
	}
	want := map[string]string{
		"psr/log":      "3.0.2",
		"vendor/alpha": "1.2.3",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d packages %v, want exactly %v (transitive and platform entries excluded)", len(got), got, want)
	}
	for name, v := range want {
		if got[name] != v {
			t.Fatalf("%s version = %q, want the exact installed %q (not the declared constraint)", name, got[name], v)
		}
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
