package prefix

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"npmitude/internal/domain"
)

// Info describes one detected Node installation prefix.
type Info struct {
	ID          string // absolute prefix path (the environment id)
	NodeVersion string
	PkgCount    int
	Source      string // "nvm" | "fnm" | "volta" | "system"
	Active      bool
}

// Config makes detection testable: HomeDir overrides the user home, LookEnv
// overrides environment lookups (NVM_DIR, FNM_DIR, VOLTA_HOME), and ActiveFn
// overrides active-prefix resolution.
type Config struct {
	HomeDir  string
	LookEnv  func(string) (string, bool)
	ActiveFn func(ctx context.Context) (string, error)
}

func (c *Config) activeFn() func(context.Context) (string, error) {
	if c.ActiveFn != nil {
		return c.ActiveFn
	}
	return Active
}

func (c *Config) home() string {
	if c.HomeDir != "" {
		return c.HomeDir
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

func (c *Config) env(key, fallback string) string {
	if c.LookEnv != nil {
		if v, ok := c.LookEnv(key); ok && v != "" {
			return v
		}
		return fallback
	}
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var versionDirRE = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)`)

// Detect finds all Node prefixes: nvm, fnm, and volta installations plus the
// active npm prefix. Each entry carries its Node version (where determinable)
// and top-level global package count. Results are unique by path, sorted by
// Node version descending.
func Detect(ctx context.Context, cfg Config) ([]Info, error) {
	var out []Info
	seen := map[string]bool{}

	add := func(id, source, version string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, Info{
			ID:          id,
			NodeVersion: version,
			PkgCount:    countPackages(filepath.Join(id, "lib", "node_modules")),
			Source:      source,
		})
	}

	nvmBase := cfg.env("NVM_DIR", filepath.Join(cfg.home(), ".nvm"))
	if nvmBase != "" {
		for _, v := range scanDirs(filepath.Join(nvmBase, "versions", "node")) {
			p := filepath.Join(nvmBase, "versions", "node", v)
			if hasNode(p) {
				add(p, "nvm", versionFromDir(v))
			}
		}
	}

	fnmBase := cfg.env("FNM_DIR", filepath.Join(cfg.home(), ".local", "share", "fnm"))
	if fnmBase != "" {
		for _, v := range scanDirs(filepath.Join(fnmBase, "node-versions")) {
			p := filepath.Join(fnmBase, "node-versions", v, "installation")
			if hasNode(p) {
				add(p, "fnm", versionFromDir(v))
			}
		}
	}

	voltaHome := cfg.env("VOLTA_HOME", filepath.Join(cfg.home(), ".volta"))
	if voltaHome != "" {
		for _, v := range scanDirs(filepath.Join(voltaHome, "tools", "image", "node")) {
			p := filepath.Join(voltaHome, "tools", "image", "node", v)
			if hasNode(p) {
				add(p, "volta", versionFromDir(v))
			}
		}
	}

	active, err := cfg.activeFn()(ctx)
	if err == nil && active != "" {
		info := Info{ID: active, Source: "system", Active: true, PkgCount: countPackages(filepath.Join(active, "lib", "node_modules"))}
		if existing, ok := findByID(out, active); ok {
			existing.Active = true
		} else {
			info.NodeVersion = nodeVersionOf(ctx, active)
			out = append(out, info)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if c := domain.CompareVersions(out[i].NodeVersion, out[j].NodeVersion); c != 0 {
			return c > 0
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func findByID(infos []Info, id string) (*Info, bool) {
	for i := range infos {
		if infos[i].ID == id {
			return &infos[i], true
		}
	}
	return nil, false
}

func scanDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func hasNode(prefixID string) bool {
	st, err := os.Stat(filepath.Join(prefixID, "bin", "node"))
	return err == nil && !st.IsDir()
}

func versionFromDir(name string) string {
	if m := versionDirRE.FindStringSubmatch(name); m != nil {
		return "v" + m[1]
	}
	return ""
}

// nodeVersionOf asks the prefix's own node for its version.
func nodeVersionOf(ctx context.Context, prefixID string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, filepath.Join(prefixID, "bin", "node"), "-p", "process.versions.node").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// countPackages counts top-level package directories (scoped @x/y count as
// one each) in a global node_modules directory.
func countPackages(modDir string) int {
	entries, err := os.ReadDir(modDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), "@") {
			subs, err := os.ReadDir(filepath.Join(modDir, e.Name()))
			if err != nil {
				continue
			}
			for _, s := range subs {
				if s.IsDir() {
					n++
				}
			}
		} else {
			n++
		}
	}
	return n
}
