package app

import (
	"os"
	"path/filepath"
	"testing"

	"npmitude/internal/ecosystem"
)

func markerStub(id, marker string) *stubEco {
	s := newStubEco().withID(id)
	s.caps = &ecosystem.Caps{ProjectScope: true}
	s.detectFn = func(root string) (bool, ecosystem.Meta) {
		_, err := os.Stat(filepath.Join(root, marker))
		return err == nil, nil
	}
	return s
}

func applicableIDs(t *testing.T, adapters []ecosystem.Ecosystem, root string) []string {
	t.Helper()
	out := DetectApplicable(adapters, root)
	ids := make([]string, 0, len(out))
	for _, a := range out {
		ids = append(ids, a.ID)
	}
	return ids
}

func TestDetectApplicableComposerFixture(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := applicableIDs(t, []ecosystem.Ecosystem{
		newStubEco().withID("npm"), // global-only: must not be consulted
		markerStub("composer", "composer.json"),
		markerStub("gomod", "go.mod"),
	}, root)
	if len(got) != 1 || got[0] != "composer" {
		t.Fatalf("applicable = %v, want [composer]", got)
	}
}

func TestDetectApplicableGomodFixture(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := applicableIDs(t, []ecosystem.Ecosystem{
		markerStub("composer", "composer.json"),
		markerStub("gomod", "go.mod"),
	}, root)
	if len(got) != 1 || got[0] != "gomod" {
		t.Fatalf("applicable = %v, want [gomod]", got)
	}
}

func TestDetectApplicableEmptyDirReportsNothing(t *testing.T) {
	got := applicableIDs(t, []ecosystem.Ecosystem{
		markerStub("composer", "composer.json"),
		markerStub("gomod", "go.mod"),
	}, t.TempDir())
	if len(got) != 0 {
		t.Fatalf("applicable = %v, want none for a marker-less directory", got)
	}
}

func TestDetectApplicableUsesVariantAsManagerID(t *testing.T) {
	node := newStubEco().withID("npm")
	node.caps = &ecosystem.Caps{GlobalScope: true, ProjectScope: true}
	node.detectFn = func(root string) (bool, ecosystem.Meta) {
		if _, err := os.Stat(filepath.Join(root, "package.json")); err != nil {
			return false, nil
		}
		return true, ecosystem.Meta{ecosystem.MetaVariant: "pnpm"}
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := applicableIDs(t, []ecosystem.Ecosystem{node}, root)
	if len(got) != 1 || got[0] != "pnpm" {
		t.Fatalf("applicable = %v, want the lockfile variant [pnpm], not the adapter id", got)
	}
}
