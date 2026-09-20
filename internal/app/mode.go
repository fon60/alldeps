package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// Mode selects the top-level scope of one launch: global package management
// or a single project directory.
type Mode int

const (
	ModeGlobal Mode = iota
	ModeProject
)

// SelectMode decides the entry mode from the optional path argument (design
// D1): no argument launches global mode; an existing directory launches
// project mode rooted at its absolute path; anything else is rejected with a
// notice and must not enter project mode.
func SelectMode(arg string) (Mode, string, error) {
	if arg == "" {
		return ModeGlobal, "", nil
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return ModeGlobal, "", fmt.Errorf("cannot resolve path %q: %v", arg, err)
	}
	fi, err := os.Stat(abs)
	if err != nil || !fi.IsDir() {
		return ModeGlobal, "", fmt.Errorf("%s is not an existing directory — project mode not started", displayPath(abs))
	}
	return ModeProject, abs, nil
}
