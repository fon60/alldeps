// Package gomod implements the ecosystem.Ecosystem port for Go module
// projects using the Go toolchain itself as the source of truth (design D1):
// listing and outdated detection through `go list -m`, mutation through
// `go get`. Project scope only — go install leaves no manifest, so there is
// no enumerable global Go tool scope.
package gomod

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"npmitude/internal/domain"
	"npmitude/internal/ecosystem"
	"npmitude/internal/lock"
)

const (
	envKind    = "go-module"
	defaultTTL = 5 * time.Minute
)

// errNoGo is the load-failure reported when no go toolchain is on PATH.
var errNoGo = errors.New(`Go toolchain required: "go" not found on PATH`)

// goModule is one line of `go list -m -f {{json}}` output; only the
// long-stable documented fields are read.
type goModule struct {
	Path     string `json:"Path"`
	Version  string `json:"Version"`
	Main     bool   `json:"Main"`
	Indirect bool   `json:"Indirect"`
	Update   *struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
	} `json:"Update"`
}

type outdatedEntry struct {
	versions  map[string]string
	fetchedAt time.Time
}

// Ecosystem is the gomod adapter. A non-empty root binds the instance to one
// Go module project directory (project scope).
type Ecosystem struct {
	root  string
	locks *lock.Manager
	TTL   time.Duration

	mu       sync.Mutex
	outdated map[string]outdatedEntry // envID -> cached update candidates
}

// New returns the gomod adapter using the default session lock directory.
func New() *Ecosystem {
	return &Ecosystem{locks: lock.NewDefault(), TTL: defaultTTL, outdated: map[string]outdatedEntry{}}
}

// NewProject returns an adapter bound to one Go module project directory.
func NewProject(root string) *Ecosystem {
	e := New()
	e.root = root
	return e
}

func (e *Ecosystem) ID() string { return "gomod" }

func (e *Ecosystem) Capabilities() ecosystem.Caps {
	// Go has no package index to search, no native lock, and MVS resolves
	// version clashes by construction, so the resolver has nothing to offer:
	// project scope only.
	return ecosystem.Caps{ProjectScope: true}
}

func (e *Ecosystem) Discover(ctx context.Context) ([]ecosystem.Environment, error) {
	return []ecosystem.Environment{{
		ID:   e.root,
		Kind: envKind,
		Meta: ecosystem.Meta{ecosystem.MetaSource: "project"},
	}}, nil
}

// DetectProject reports whether root is a Go module project (a go.mod file
// present). It is a pure filesystem read; no toolchain binary runs.
func (e *Ecosystem) DetectProject(root string) (bool, ecosystem.Meta) {
	fi, err := os.Stat(filepath.Join(root, "go.mod"))
	if err != nil || !fi.Mode().IsRegular() {
		return false, nil
	}
	return true, nil
}

// goBin resolves the go toolchain from PATH (design D6); absence yields a
// clear load-failure naming the missing binary.
func goBin() (string, error) {
	bin, err := exec.LookPath("go")
	if err != nil {
		return "", errNoGo
	}
	return bin, nil
}

// listModules runs `go list -m [-u] -json all` in the project directory and
// parses the stream of module objects it prints (one per module). A parse
// failure is a load failure, never partial state.
func (e *Ecosystem) listModules(ctx context.Context, updates bool) ([]goModule, error) {
	bin, err := goBin()
	if err != nil {
		return nil, err
	}
	args := []string{"list", "-m"}
	if updates {
		args = append(args, "-u")
	}
	args = append(args, "-json", "all")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = e.root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	var out []goModule
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var m goModule
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("go list: cannot parse module JSON: %w", err)
		}
		out = append(out, m)
	}
	return out, nil
}

func (e *Ecosystem) ListInstalled(ctx context.Context, env ecosystem.Environment) ([]ecosystem.Package, error) {
	modules, err := e.listModules(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]ecosystem.Package, 0, len(modules))
	for _, m := range modules {
		if m.Main || m.Indirect {
			continue
		}
		out = append(out, ecosystem.Package{Name: m.Path, Version: m.Version})
	}
	return out, nil
}

func (e *Ecosystem) LatestVersions(ctx context.Context, env ecosystem.Environment, names []string) (map[string]string, int, error) {
	e.mu.Lock()
	if entry, ok := e.outdated[env.ID]; ok && time.Since(entry.fetchedAt) < e.TTL {
		e.mu.Unlock()
		return filterNames(entry.versions, names), 0, nil
	}
	e.mu.Unlock()

	modules, err := e.listModules(ctx, true)
	if err != nil {
		return nil, 0, err
	}
	all := map[string]string{}
	for _, m := range modules {
		if m.Main || m.Indirect {
			continue
		}
		if m.Update != nil && m.Update.Version != "" && m.Update.Version != m.Version {
			all[m.Path] = m.Update.Version
		}
	}
	e.mu.Lock()
	e.outdated[env.ID] = outdatedEntry{versions: all, fetchedAt: time.Now()}
	e.mu.Unlock()
	return filterNames(all, names), 0, nil
}

func filterNames(all map[string]string, names []string) map[string]string {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	out := map[string]string{}
	for name, v := range all {
		if want[name] {
			out[name] = v
		}
	}
	return out
}

func (e *Ecosystem) Search(ctx context.Context, env ecosystem.Environment, query string, size, from int) ([]ecosystem.Hit, int, error) {
	return nil, 0, errors.New("search is not available for Go modules")
}

// goGetArgs renders one `go get` invocation: install/upgrade targets carry an
// explicit version (latest when unpinned), removals use @none.
func goGetArgs(op ecosystem.OpKind, items []ecosystem.Item) []string {
	args := []string{"get"}
	for _, it := range items {
		switch {
		case op == ecosystem.OpRemove:
			args = append(args, it.Name+"@none")
		case it.Version != "":
			args = append(args, it.Name+"@"+it.Version)
		default:
			args = append(args, it.Name+"@latest")
		}
	}
	return args
}

// Resolve batches the intent's operations by op kind. Go has no conflict
// resolution (MVS resolves clashes by construction): it always yields a plan
// and an empty conflict set.
func (e *Ecosystem) Resolve(intent ecosystem.Intent) (ecosystem.Plan, []ecosystem.Conflict, error) {
	var batches []ecosystem.Batch
	for _, op := range []ecosystem.OpKind{ecosystem.OpInstall, ecosystem.OpUpgrade, ecosystem.OpRemove} {
		var items []ecosystem.Item
		for _, it := range intent.Items {
			if it.Op != op {
				continue
			}
			items = append(items, ecosystem.Item{Name: it.Name, Version: it.Version})
		}
		if len(items) == 0 {
			continue
		}
		batches = append(batches, ecosystem.Batch{Op: op, Items: items, Label: "go " + strings.Join(goGetArgs(op, items), " ")})
	}
	return ecosystem.Plan{Env: intent.Env, Batches: batches}, nil, nil
}

// Execute runs one plan batch with `go get` rooted in the project directory
// and returns its combined raw output. The output is not interpreted; truth
// comes from a post-run ListInstalled. Removals are surgical (@none drops the
// requirement without touching unrelated ones — no automatic go mod tidy).
func (e *Ecosystem) Execute(ctx context.Context, env ecosystem.Environment, batch ecosystem.Batch) (string, error) {
	bin, err := goBin()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, bin, goGetArgs(batch.Op, batch.Items)...)
	cmd.Dir = e.root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e *Ecosystem) UnpackedSize(ctx context.Context, env ecosystem.Environment, name, version string) (int64, error) {
	return 0, nil
}

// moduleVersions enumerates a module's tagged versions through
// `go list -m -versions`, which prints the module path followed by every
// tagged version, space-separated on one line. It runs in the project
// directory so the module context (and any GOFLAGS/proxy config) applies.
func (e *Ecosystem) moduleVersions(ctx context.Context, name string) ([]string, error) {
	bin, err := goBin()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, "list", "-m", "-versions", name)
	cmd.Dir = e.root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list -m -versions: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	fields := strings.Fields(stdout.String())
	if len(fields) < 2 {
		return nil, fmt.Errorf("go list -m -versions: no tagged versions for %s", name)
	}
	return fields[1:], nil
}

// Doc reports the module's published (tagged) versions so the version
// history works for Go dependencies; there is no richer package document.
func (e *Ecosystem) Doc(ctx context.Context, env ecosystem.Environment, name string, installed bool) (*ecosystem.Doc, bool, error) {
	versions, err := e.moduleVersions(ctx, name)
	if err != nil {
		return nil, false, err
	}
	domain.SortVersions(versions)
	doc := &ecosystem.Doc{Name: name, Versions: versions}
	if len(versions) > 0 {
		doc.Latest = versions[0]
	}
	return doc, false, nil
}

func (e *Ecosystem) Readme(env ecosystem.Environment, name string) (string, bool) {
	return "", false
}

// Writable reports whether the project directory exists and is writable.
func (e *Ecosystem) Writable(env ecosystem.Environment) bool {
	if e.root == "" {
		return false
	}
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

// Lock acquires the environment lock. Go has no native lock of its own, so
// this always takes the session-scoped fallback path.
func (e *Ecosystem) Lock(env ecosystem.Environment) (ecosystem.LockHandle, error) {
	if err := e.locks.Acquire(env.ID); err != nil {
		return nil, err
	}
	return &lockHandle{mgr: e.locks, envID: env.ID}, nil
}
