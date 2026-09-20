package gomod

import (
	"context"
	"strings"
	"testing"

	"npmitude/internal/ecosystem"
)

func TestListInstalledMissingGoToolchain(t *testing.T) {
	t.Setenv("PATH", "")
	root := t.TempDir()
	e := NewProject(root)

	_, err := e.ListInstalled(context.Background(), ecosystem.Environment{ID: root})
	if err == nil {
		t.Fatal("expected a load failure when no go toolchain is on PATH")
	}
	msg := err.Error()
	if !strings.Contains(msg, "go") {
		t.Fatalf("error must name the missing binary: %q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "not found on path") {
		t.Fatalf("error must state the toolchain is not on PATH: %q", msg)
	}

	if _, _, err := e.LatestVersions(context.Background(), ecosystem.Environment{ID: root}, nil); err == nil || !strings.Contains(err.Error(), "go") {
		t.Fatalf("LatestVersions must fail with the same clear error, got %v", err)
	}
	if _, err := e.Execute(context.Background(), ecosystem.Environment{ID: root},
		ecosystem.Batch{Op: ecosystem.OpInstall, Items: []ecosystem.Item{{Name: "x"}}}); err == nil || !strings.Contains(err.Error(), "go") {
		t.Fatalf("Execute must fail with the same clear error, got %v", err)
	}
}
