package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectModeNoArgIsGlobal(t *testing.T) {
	mode, root, err := SelectMode("")
	if err != nil {
		t.Fatalf("no argument must not be rejected: %v", err)
	}
	if mode != ModeGlobal {
		t.Fatalf("mode = %v, want ModeGlobal", mode)
	}
	if root != "" {
		t.Fatalf("root = %q, want empty for global mode", root)
	}
}

func TestSelectModeDotIsProject(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	mode, root, err := SelectMode(".")
	if err != nil {
		t.Fatalf("npmitude . must select project mode: %v", err)
	}
	if mode != ModeProject {
		t.Fatalf("mode = %v, want ModeProject", mode)
	}
	if abs, _ := filepath.Abs(dir); root != abs {
		t.Fatalf("root = %q, want %q", root, abs)
	}
}

func TestSelectModeExistingDirIsProject(t *testing.T) {
	dir := t.TempDir()
	mode, root, err := SelectMode(dir)
	if err != nil {
		t.Fatalf("existing directory must select project mode: %v", err)
	}
	if mode != ModeProject || root == "" {
		t.Fatalf("mode = %v root = %q, want project mode with a root", mode, root)
	}

	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	mode, _, err = SelectMode("./sub")
	if err != nil || mode != ModeProject {
		t.Fatalf("relative existing directory must select project mode: %v (%v)", mode, err)
	}
}

func TestSelectModeRejectsMissingPath(t *testing.T) {
	mode, _, err := SelectMode(filepath.Join(t.TempDir(), "no-such-dir"))
	if err == nil {
		t.Fatal("a missing path must be rejected with a notice")
	}
	if mode == ModeProject {
		t.Fatal("a rejected path must not enter project mode")
	}
}

func TestSelectModeRejectsFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mode, _, err := SelectMode(f)
	if err == nil {
		t.Fatal("a path naming a file must be rejected with a notice")
	}
	if mode == ModeProject {
		t.Fatal("a file path must not enter project mode")
	}
}
