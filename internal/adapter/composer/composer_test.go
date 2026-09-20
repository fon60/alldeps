package composer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"npmitude/internal/ecosystem"
)

func TestDetectProjectComposerJSON(t *testing.T) {
	root := t.TempDir()
	if ok, meta := New().DetectProject(root); ok {
		t.Fatalf("DetectProject on an empty dir = true (meta %v), want not applicable", meta)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require":{"psr/log":"^3.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, meta := New().DetectProject(root)
	if !ok {
		t.Fatal("DetectProject with a composer.json present = false, want applicable")
	}
	if len(meta) != 0 {
		t.Fatalf("meta = %v, want none (no variant for Composer)", meta)
	}

	dirAsManifest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dirAsManifest, "composer.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if ok, _ := New().DetectProject(dirAsManifest); ok {
		t.Fatal("a composer.json directory must not count as a Composer project")
	}
}

func TestCapabilitiesProjectScopeSearchConflictResolution(t *testing.T) {
	caps := New().Capabilities()
	for name, v := range map[string]bool{
		"ProjectScope":          caps.ProjectScope,
		"HasSearch":             caps.HasSearch,
		"HasConflictResolution": caps.HasConflictResolution,
	} {
		if !v {
			t.Fatalf("capability %s must be true for Composer", name)
		}
	}
	for name, v := range map[string]bool{
		"GlobalScope":   caps.GlobalScope,
		"HasDedupe":     caps.HasDedupe,
		"HasNativeLock": caps.HasNativeLock,
	} {
		if v {
			t.Fatalf("capability %s must be false for Composer", name)
		}
	}
}

func TestDiscoverReturnsProjectEnvironment(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
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
		t.Fatalf("env kind = %q, want %q", e.Kind, "composer-project")
	}
	if e.Meta[ecosystem.MetaSource] != "project" {
		t.Fatalf("meta source = %q, want project", e.Meta[ecosystem.MetaSource])
	}
}
