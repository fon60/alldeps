// Package sizes measures on-disk usage of globally installed packages by
// walking the prefix's global module directory (design D2: sizes are not in
// npm ls output and render progressively as "…" until measured).
package sizes

import (
	"io/fs"
	"path/filepath"
	"syscall"
)

type inodeKey struct {
	dev uint64
	ino uint64
}

func inodeOf(info fs.FileInfo) (inodeKey, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return inodeKey{}, false
	}
	return inodeKey{dev: st.Dev, ino: st.Ino}, true
}

// Measure returns the total apparent byte size of regular files under dir,
// counting hard-linked inodes only once (matching du). Unreadable entries
// are skipped; symlinks are not followed.
func Measure(dir string) (int64, error) {
	var total int64
	seen := map[inodeKey]bool{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if key, ok := inodeOf(info); ok {
			if seen[key] {
				return nil
			}
			seen[key] = true
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

// GlobalModuleDir returns the global node_modules directory of a prefix.
func GlobalModuleDir(prefixID string) string {
	return filepath.Join(prefixID, "lib", "node_modules")
}

// PackageDir returns the on-disk directory of one top-level global package.
func PackageDir(prefixID, name string) string {
	return filepath.Join(GlobalModuleDir(prefixID), name)
}
