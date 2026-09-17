package sizes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMeasureKnownTree(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), 100)
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(sub, "b.js"), 250)
	mustWrite(t, filepath.Join(sub, "c.json"), 50)

	got, err := Measure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != 400 {
		t.Fatalf("measured %d bytes, want 400", got)
	}
}

func TestMeasureSymlinkNotFollowed(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "big.bin"), 10000)
	mustWrite(t, filepath.Join(dir, "real.txt"), 10)
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	got, err := Measure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != 10 {
		t.Fatalf("measured %d bytes, want 10 (symlink target excluded)", got)
	}
}

func TestMeasureHardLinksCountedOnce(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig.bin")
	mustWrite(t, orig, 5000)
	link := filepath.Join(dir, "link.bin")
	if err := os.Link(orig, link); err != nil {
		t.Skipf("cannot create hard link: %v", err)
	}

	got, err := Measure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != 5000 {
		t.Fatalf("measured %d bytes, want 5000 (hard link counted once)", got)
	}
}

func TestMeasureMissingDir(t *testing.T) {
	if _, err := Measure(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func mustWrite(t *testing.T, path string, size int) {
	t.Helper()
	buf := make([]byte, size)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}
