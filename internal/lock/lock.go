package lock

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Info identifies the holder recorded in a lock file.
type Info struct {
	Prefix string // absolute prefix path (human readable)
	PID    int
	Start  int64 // Unix nanoseconds at which the holder process started
	Host   string
}

// HeldError is returned when an environment is held by a live instance.
type HeldError struct {
	Holder Info
}

func (e *HeldError) Error() string {
	return fmt.Sprintf("environment %s is held by pid %d on %s", e.Holder.Prefix, e.Holder.PID, e.Holder.Host)
}

// maxAge is the backstop for locks whose holder cannot be verified (foreign
// host). Liveness checks clear dead holders immediately; this only bounds how
// long an unverifiable lock can block an environment.
const maxAge = 24 * time.Hour

// startTolerance bounds the difference between a recorded holder start time
// and the process's actual start time when verifying PID identity (dodging
// PID reuse).
const startTolerance = 2 * time.Minute

// Manager acquires and releases per-environment locks under one directory.
type Manager struct {
	dir      string
	held     map[string]bool // envID -> held by this instance
	start    int64           // this process's start time (Unix nanoseconds)
	liveness func(Info) bool
}

// NewDefault returns a Manager using $XDG_STATE_HOME/npmitude/locks (default
// ~/.local/state/npmitude/locks).
func NewDefault() *Manager {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = "."
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return New(filepath.Join(dir, "npmitude", "locks"))
}

// New returns a Manager that stores lock files under dir.
func New(dir string) *Manager {
	start, ok := processStartUnixNano(os.Getpid())
	if !ok {
		start = time.Now().UnixNano()
	}
	return &Manager{dir: dir, held: map[string]bool{}, start: start, liveness: defaultLiveness}
}

// ID is the short hash of an absolute prefix path used as the lock file name.
func ID(prefixID string) string {
	sum := sha256.Sum256([]byte(prefixID))
	return hex.EncodeToString(sum[:])[:12]
}

// Acquire takes the lock for prefixID. It returns *HeldError when a live
// instance holds it, nil on success (including when we already hold it).
func (m *Manager) Acquire(prefixID string) error {
	id := ID(prefixID)
	if m.held[id] {
		return nil
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(m.dir, id+".lock")
	for attempt := 0; attempt < 5; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			info := Info{Prefix: prefixID, PID: os.Getpid(), Start: m.start, Host: hostname()}
			if _, werr := f.WriteString(formatInfo(info)); werr != nil {
				f.Close()
				os.Remove(path)
				return werr
			}
			f.Close()
			m.held[id] = true
			return nil
		}
		if !os.IsExist(err) {
			return err
		}
		existing, perr := readLock(path)
		if perr == nil && m.liveness(existing) {
			return &HeldError{Holder: existing}
		}
		os.Remove(path) // stale or unreadable: clear and retry
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("could not acquire lock (contention)")
}

// Release drops our lock for prefixID. It is a no-op when we do not hold it.
func (m *Manager) Release(prefixID string) {
	id := ID(prefixID)
	if !m.held[id] {
		return
	}
	delete(m.held, id)
	os.Remove(filepath.Join(m.dir, id+".lock"))
}

// ReleaseAll drops every lock held by this instance (called on exit).
func (m *Manager) ReleaseAll() {
	for id := range m.held {
		os.Remove(filepath.Join(m.dir, id+".lock"))
	}
	m.held = map[string]bool{}
}

// IsHeld reports whether prefixID is held by a live instance (ours or
// another's). Stale locks read as not held; they are cleared on the next
// Acquire.
func (m *Manager) IsHeld(prefixID string) bool {
	id := ID(prefixID)
	if m.held[id] {
		return true
	}
	info, err := readLock(filepath.Join(m.dir, id+".lock"))
	if err != nil {
		return false
	}
	return m.liveness(info)
}

func (m *Manager) lockPath(prefixID string) string {
	return filepath.Join(m.dir, ID(prefixID)+".lock")
}

func defaultLiveness(info Info) bool {
	if info.PID <= 0 {
		return false
	}
	host, _ := os.Hostname()
	if info.Host != host {
		return time.Since(time.Unix(0, info.Start)) < maxAge
	}
	if err := syscall.Kill(info.PID, 0); err != nil {
		return false
	}
	actual, ok := processStartUnixNano(info.PID)
	if !ok {
		return true // alive but start unverifiable: assume held (refuse, don't take over)
	}
	d := actual - info.Start
	if d < 0 {
		d = -d
	}
	return d < int64(startTolerance)
}

func formatInfo(i Info) string {
	return fmt.Sprintf("path=%s\npid=%d\nstart=%d\nhost=%s\n", i.Prefix, i.PID, i.Start, i.Host)
}

func readLock(path string) (Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	var info Info
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "path":
			info.Prefix = v
		case "pid":
			pid, err := strconv.Atoi(v)
			if err != nil {
				return Info{}, errors.New("bad pid in lock file")
			}
			info.PID = pid
		case "start":
			start, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return Info{}, errors.New("bad start in lock file")
			}
			info.Start = start
		case "host":
			info.Host = v
		}
	}
	if info.PID <= 0 || info.Start == 0 {
		return Info{}, errors.New("incomplete lock file")
	}
	return info, nil
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}

// processStartUnixNano reads a process's start time from /proc in Unix
// nanoseconds.
func processStartUnixNano(pid int) (int64, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	s := string(data)
	i := strings.LastIndexByte(s, ')') // comm may contain spaces and parens
	if i < 0 {
		return 0, false
	}
	fields := strings.Fields(s[i+1:])
	if len(fields) < 20 {
		return 0, false
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64) // field 22: starttime
	if err != nil {
		return 0, false
	}
	const hz = 100 // USER_HZ on Linux
	btime, ok := bootTimeSecs()
	if !ok {
		return 0, false
	}
	return btime*int64(time.Second) + ticks*int64(time.Second)/hz, true
}

func bootTimeSecs() (int64, bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			v, err := strconv.ParseInt(strings.TrimSpace(line[len("btime "):]), 10, 64)
			if err != nil {
				return 0, false
			}
			return v, true
		}
	}
	return 0, false
}
