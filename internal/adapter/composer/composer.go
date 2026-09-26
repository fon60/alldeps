package composer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"npmitude/internal/ecosystem"
	"npmitude/internal/lock"
)

const (
	envKind    = "composer-project"
	defaultTTL = 5 * time.Minute
)

// errNoComposer is the load-failure reported when no composer binary is on PATH.
var errNoComposer = errors.New(`Composer toolchain required: "composer" not found on PATH`)

// Ecosystem is the composer adapter, bound to one Composer project directory
// (project scope only).
type Ecosystem struct {
	root      string
	locks     *lock.Manager
	packagist *Packagist

	mu       sync.Mutex
	phpVer   string
	phpCheck bool
}

// New returns the composer adapter using the default session lock directory.
func New() *Ecosystem {
	return &Ecosystem{locks: lock.NewDefault(), packagist: NewPackagist()}
}

// NewProject returns an adapter bound to one Composer project directory.
func NewProject(root string) *Ecosystem {
	e := New()
	e.root = root
	return e
}

func (e *Ecosystem) ID() string { return "composer" }

func (e *Ecosystem) Capabilities() ecosystem.Caps {
	// Project scope only: COMPOSER_HOME is out of scope, vendor is per-project
	// (no cross-destination redundancy), composer.lock is data not a mutex.
	// Packagist is searchable and the solver genuinely reports conflicts.
	return ecosystem.Caps{ProjectScope: true, HasSearch: true, HasConflictResolution: true}
}

func (e *Ecosystem) Discover(ctx context.Context) ([]ecosystem.Environment, error) {
	return []ecosystem.Environment{{
		ID:   e.root,
		Kind: envKind,
		Meta: ecosystem.Meta{ecosystem.MetaSource: "project"},
	}}, nil
}

// DetectProject reports whether root is a Composer project (a composer.json
// file present). It is a pure filesystem read; no toolchain binary runs.
func (e *Ecosystem) DetectProject(root string) (bool, ecosystem.Meta) {
	fi, err := os.Stat(filepath.Join(root, "composer.json"))
	if err != nil || !fi.Mode().IsRegular() {
		return false, nil
	}
	return true, nil
}

type installedJSON struct {
	Packages []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"packages"`
}

// ListInstalled shows every installed package from vendor/composer/installed.json
// with its exact version: a name declared in composer.json require is direct,
// the rest were pulled in automatically. Platform/virtual requirements (php,
// ext-*) are not installed packages and never appear as rows. A missing vendor
// directory yields an empty list — operations still work, composer creates
// vendor on first require.
func (e *Ecosystem) ListInstalled(ctx context.Context, env ecosystem.Environment) ([]ecosystem.Package, error) {
	direct, err := directRequires(e.root)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(e.root, "vendor", "composer", "installed.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return []ecosystem.Package{}, nil
	}
	if err != nil {
		return nil, err
	}
	var inst installedJSON
	if err := json.Unmarshal(data, &inst); err != nil {
		return nil, fmt.Errorf("composer: cannot parse vendor/composer/installed.json: %w", err)
	}
	out := make([]ecosystem.Package, 0, len(inst.Packages))
	for _, p := range inst.Packages {
		if isPlatformRequirement(p.Name) {
			continue
		}
		out = append(out, ecosystem.Package{Name: p.Name, Version: p.Version, Automatic: !direct[p.Name]})
	}
	return out, nil
}

// isPlatformRequirement reports whether name is a composer platform/virtual
// requirement (php, ext-*) rather than an installable package.
func isPlatformRequirement(name string) bool {
	return name == "php" || strings.HasPrefix(name, "ext-")
}

// directRequires returns the keys of composer.json's require section.
func directRequires(root string) (map[string]bool, error) {
	data, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return nil, fmt.Errorf("composer: cannot read composer.json: %w", err)
	}
	var cj struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(data, &cj); err != nil {
		return nil, fmt.Errorf("composer: cannot parse composer.json: %w", err)
	}
	out := make(map[string]bool, len(cj.Require))
	for name := range cj.Require {
		out[name] = true
	}
	return out, nil
}

func (e *Ecosystem) Search(ctx context.Context, env ecosystem.Environment, query string, size, from int) ([]ecosystem.Hit, int, error) {
	hits, total, err := e.packagist.Search(ctx, query, size, from)
	if err != nil {
		return nil, 0, err
	}
	names := make([]string, 0, len(hits))
	for _, h := range hits {
		names = append(names, h.Name)
	}
	latest, _ := e.packagist.LatestFor(ctx, names)
	out := make([]ecosystem.Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, ecosystem.Hit{Name: h.Name, Version: latest[h.Name], Description: h.Description})
	}
	return out, total, nil
}

func (e *Ecosystem) LatestVersions(ctx context.Context, env ecosystem.Environment, names []string) (map[string]string, int, error) {
	versions, failed := e.packagist.LatestFor(ctx, names)
	return versions, failed, nil
}

// composerBin resolves the composer toolchain from PATH; absence yields a
// clear load-failure naming the missing binary.
func composerBin() (string, error) {
	bin, err := exec.LookPath("composer")
	if err != nil {
		return "", errNoComposer
	}
	return bin, nil
}

// composerArgs renders one composer invocation for an op kind and its items:
// install/upgrade map to `require name:constraint` (bare name = latest),
// removals to `remove name`. Interactivity is suppressed but lifecycle scripts
// run normally — no --no-scripts.
func composerArgs(op ecosystem.OpKind, items []ecosystem.Item) []string {
	verb := "require"
	if op == ecosystem.OpRemove {
		verb = "remove"
	}
	args := []string{verb}
	for _, it := range items {
		switch {
		case op == ecosystem.OpRemove:
			args = append(args, it.Name)
		case it.Version != "":
			args = append(args, it.Name+":"+it.Version)
		default:
			args = append(args, it.Name)
		}
	}
	return append(args, "--no-interaction")
}

// runComposer runs one composer invocation in the project directory and
// returns its combined output (stdout and stderr together).
func (e *Ecosystem) runComposer(args ...string) (string, error) {
	bin, err := composerBin()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = e.root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Execute runs one plan batch with composer rooted in the project directory
// and returns its combined raw output. The output is not interpreted; truth
// comes from a post-run ListInstalled.
func (e *Ecosystem) Execute(ctx context.Context, env ecosystem.Environment, batch ecosystem.Batch) (string, error) {
	bin, err := composerBin()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, bin, composerArgs(batch.Op, batch.Items)...)
	cmd.Dir = e.root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e *Ecosystem) UnpackedSize(ctx context.Context, env ecosystem.Environment, name, version string) (int64, error) {
	// Packagist carries no unpacked sizes.
	return 0, nil
}

func (e *Ecosystem) Doc(ctx context.Context, env ecosystem.Environment, name string, installed bool) (*ecosystem.Doc, bool, error) {
	if installed {
		if doc, ok, err := e.localDoc(name); err == nil {
			return doc, ok, nil
		}
	}
	vs, err := e.packagist.Metadata(ctx, name)
	if err != nil {
		return nil, false, err
	}
	doc := &ecosystem.Doc{Name: name, Bin: map[string]string{}}
	for _, v := range vs {
		doc.Versions = append(doc.Versions, v.Version)
	}
	if best := latestStable(vs); best != "" {
		doc.Latest = best
		for _, v := range vs {
			if v.Version == best {
				doc.Description = v.Description
				doc.License = v.License
				doc.Homepage = v.Homepage
				doc.Repository = v.SourceURL
				break
			}
		}
	}
	return doc, false, nil
}

// localDoc builds the info document from vendor/<name>/composer.json so the
// info screen works offline for installed packages.
func (e *Ecosystem) localDoc(name string) (*ecosystem.Doc, bool, error) {
	data, err := os.ReadFile(filepath.Join(e.root, "vendor", name, "composer.json"))
	if err != nil {
		return nil, false, err
	}
	var cj struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Version     string            `json:"version"`
		Homepage    string            `json:"homepage"`
		License     any               `json:"license"`
		Require     map[string]string `json:"require"`
	}
	if err := json.Unmarshal(data, &cj); err != nil {
		return nil, false, fmt.Errorf("composer: cannot parse vendor/%s/composer.json: %w", name, err)
	}
	doc := &ecosystem.Doc{Name: cj.Name, Description: cj.Description, Homepage: cj.Homepage, Latest: cj.Version, Dependencies: cj.Require, Bin: map[string]string{}}
	switch l := cj.License.(type) {
	case string:
		doc.License = l
	case map[string]any:
		if t, ok := l["type"].(string); ok {
			doc.License = t
		}
	}
	return doc, true, nil
}

// Readme returns the README of an installed package from its vendor directory.
func (e *Ecosystem) Readme(env ecosystem.Environment, name string) (string, bool) {
	base := filepath.Join(e.root, "vendor", name)
	for _, f := range []string{"README.md", "readme.md", "README.rst", "README.txt", "README"} {
		if data, err := os.ReadFile(filepath.Join(base, f)); err == nil {
			return string(data), true
		}
	}
	return "", false
}

// Writable reports whether the project directory exists and is writable.
func (e *Ecosystem) Writable(env ecosystem.Environment) bool {
	fi, err := os.Stat(e.root)
	if err != nil || !fi.IsDir() {
		return false
	}
	f, err := os.CreateTemp(e.root, ".npmitude-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

type lockHandle struct {
	mgr   *lock.Manager
	envID string
}

func (h *lockHandle) Release() { h.mgr.Release(h.envID) }

// Lock acquires the environment lock. Composer has no native lock of its own
// (composer.lock is data, not a mutex), so this always takes the session-
// scoped fallback path.
func (e *Ecosystem) Lock(env ecosystem.Environment) (ecosystem.LockHandle, error) {
	if err := e.locks.Acquire(env.ID); err != nil {
		return nil, err
	}
	return &lockHandle{mgr: e.locks, envID: env.ID}, nil
}

// phpVersion detects the platform PHP version once (design D4); "" when no
// php binary is on PATH.
func (e *Ecosystem) phpVersion() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.phpCheck {
		return e.phpVer
	}
	e.phpCheck = true
	if bin, err := exec.LookPath("php"); err == nil {
		if out, err := exec.Command(bin, "-r", "echo PHP_VERSION;").Output(); err == nil {
			e.phpVer = strings.TrimSpace(string(out))
		}
	}
	return e.phpVer
}
