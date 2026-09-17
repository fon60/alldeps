package lock

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func writeLock(t *testing.T, dir, prefixID string, info Info) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ID(prefixID)+".lock"), []byte(formatInfo(info)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.Acquire("/p"); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if !m.IsHeld("/p") {
		t.Fatal("IsHeld should be true for our own lock")
	}
	m.Release("/p")
	if _, err := os.Stat(filepath.Join(dir, ID("/p")+".lock")); !os.IsNotExist(err) {
		t.Fatal("lock file should be removed on release")
	}
}

func TestRefuseLiveHolder(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	writeLock(t, dir, "/p", Info{Prefix: "/p", PID: os.Getpid(), Start: m.start, Host: hostname()})

	err := m.Acquire("/p")
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("acquire err = %v, want *HeldError", err)
	}
	if held.Holder.PID != os.Getpid() {
		t.Fatalf("holder pid = %d, want ours", held.Holder.PID)
	}
}

func TestStaleClearedDeadPID(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)

	cmd := exec.Command("sleep", "0.2")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	start, ok := processStartUnixNano(pid)
	if !ok {
		t.Fatal("could not read start time of live child")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	writeLock(t, dir, "/p", Info{Prefix: "/p", PID: pid, Start: start, Host: hostname()})
	if m.IsHeld("/p") {
		t.Fatal("a dead holder's lock must not read as held")
	}
	if err := m.Acquire("/p"); err != nil {
		t.Fatalf("acquire over stale lock: %v", err)
	}
}

func TestStaleClearedReusedPID(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	// Our own live PID but a start time an hour in the past: a reused PID is
	// not the original holder.
	writeLock(t, dir, "/p", Info{Prefix: "/p", PID: os.Getpid(), Start: m.start - int64(time.Hour), Host: hostname()})
	if err := m.Acquire("/p"); err != nil {
		t.Fatalf("acquire over reused-PID lock: %v (start-time check failed)", err)
	}
}

func TestForeignHostBackstop(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)

	writeLock(t, dir, "/a", Info{Prefix: "/a", PID: 4242, Start: time.Now().UnixNano(), Host: "otherhost"})
	if !m.IsHeld("/a") {
		t.Fatal("fresh foreign-host lock should read as held (backstop not expired)")
	}

	writeLock(t, dir, "/b", Info{Prefix: "/b", PID: 4242, Start: time.Now().Add(-25 * time.Hour).UnixNano(), Host: "otherhost"})
	if m.IsHeld("/b") {
		t.Fatal("foreign-host lock older than the backstop must read as stale")
	}
}

func TestAcquireIdempotent(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.Acquire("/p"); err != nil {
		t.Fatal(err)
	}
	if err := m.Acquire("/p"); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	m.ReleaseAll()
	if len(m.held) != 0 {
		t.Fatal("ReleaseAll should drop every held lock")
	}
}
