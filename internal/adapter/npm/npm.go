// Package npm implements the ecosystem.Ecosystem port for the Node/npm
// ecosystem by delegating to the node-ecosystem implementation details
// (npmcmd, registry, prefix, sizes, lock). All npm-specific knowledge — op
// verbs, prefix layout, registry protocol, prefix detection — lives here.
package npm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"npmitude/internal/ecosystem"
	"npmitude/internal/lock"
	"npmitude/internal/npmcmd"
	"npmitude/internal/prefix"
	"npmitude/internal/registry"
)

const envKind = "node-prefix"

// opVerbs are npm's operation verbs for each manager-agnostic op kind.
var opVerbs = map[ecosystem.OpKind][]string{
	ecosystem.OpInstall: {"i", "-g"},
	ecosystem.OpUpgrade: {"i", "-g"},
	ecosystem.OpRemove:  {"rm", "-g"},
}

// Ecosystem is the npm adapter. It is safe for concurrent use; registry URLs
// are resolved once per environment and cached.
type Ecosystem struct {
	mu       sync.Mutex
	regCache map[string]string // envID -> configured registry URL
	locks    *lock.Manager
}

// New returns the npm adapter using the default session lock directory.
func New() *Ecosystem {
	return &Ecosystem{regCache: map[string]string{}, locks: lock.NewDefault()}
}

func (e *Ecosystem) ID() string { return "npm" }

func (e *Ecosystem) Capabilities() ecosystem.Caps {
	// npm has no global lock and no conflict resolution; it operates on the
	// global scope of a Node prefix only.
	return ecosystem.Caps{GlobalScope: true}
}

func (e *Ecosystem) Discover(ctx context.Context) ([]ecosystem.Environment, error) {
	infos, err := prefix.Detect(ctx, prefix.Config{})
	if err != nil {
		return nil, err
	}
	out := make([]ecosystem.Environment, 0, len(infos))
	for _, info := range infos {
		meta := ecosystem.Meta{
			ecosystem.MetaSource:   info.Source,
			ecosystem.MetaPkgCount: strconv.Itoa(info.PkgCount),
		}
		if info.Active {
			meta[ecosystem.MetaActive] = "1"
		}
		out = append(out, ecosystem.Environment{ID: info.ID, Kind: envKind, Rank: info.NodeVersion, Meta: meta})
	}
	return out, nil
}

func (e *Ecosystem) ListInstalled(ctx context.Context, env ecosystem.Environment) ([]ecosystem.Package, error) {
	parsed, err := npmcmd.LSGlobal(ctx, env.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ecosystem.Package, 0, len(parsed))
	for _, p := range parsed {
		out = append(out, ecosystem.Package{Name: p.Name, Version: p.Version, Unhealthy: p.Broken})
	}
	return out, nil
}

// registry resolves (and caches) the environment's configured registry URL.
func (e *Ecosystem) registry(ctx context.Context, envID string) (string, error) {
	e.mu.Lock()
	if url, ok := e.regCache[envID]; ok {
		e.mu.Unlock()
		return url, nil
	}
	e.mu.Unlock()
	url, err := npmcmd.GetRegistry(ctx, envID)
	if err != nil {
		return "", ecosystem.ErrNoRegistry
	}
	e.mu.Lock()
	e.regCache[envID] = url
	e.mu.Unlock()
	return url, nil
}

func (e *Ecosystem) Search(ctx context.Context, env ecosystem.Environment, query string, size, from int) ([]ecosystem.Hit, int, error) {
	url, err := e.registry(ctx, env.ID)
	if err != nil {
		return nil, 0, err
	}
	hits, total, err := registry.NewClient(url).Search(ctx, query, size, from)
	if err != nil {
		return nil, 0, err
	}
	out := make([]ecosystem.Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, ecosystem.Hit{Name: h.Name, Version: h.Version, Description: h.Description})
	}
	return out, total, nil
}

func (e *Ecosystem) LatestVersions(ctx context.Context, env ecosystem.Environment, names []string) (map[string]string, int, error) {
	url, err := e.registry(ctx, env.ID)
	if err != nil {
		return nil, 0, err
	}
	versions, failed := registry.NewClient(url).CheckOutdated(ctx, names)
	return versions, failed, nil
}

func (e *Ecosystem) UnpackedSize(ctx context.Context, env ecosystem.Environment, name, version string) (int64, error) {
	url, err := e.registry(ctx, env.ID)
	if err != nil {
		return 0, err
	}
	return registry.NewClient(url).UnpackedSize(ctx, name, version)
}

func (e *Ecosystem) Doc(ctx context.Context, env ecosystem.Environment, name string, installed bool) (*ecosystem.Doc, bool, error) {
	if installed {
		doc, err := npmcmd.LocalDoc(env.ID, name)
		if err != nil {
			return nil, false, err
		}
		return toEcoDoc(doc), true, nil
	}
	url, err := e.registry(ctx, env.ID)
	if err != nil {
		return nil, false, err
	}
	doc, err := registry.NewClient(url).GetDoc(ctx, name)
	if err != nil {
		return nil, false, err
	}
	return toEcoDoc(doc), false, nil
}

func (e *Ecosystem) Readme(env ecosystem.Environment, name string) (string, bool) {
	return npmcmd.Readme(env.ID, name)
}

// Writable reports whether the prefix is writable. Missing directories are
// allowed — npm creates them on first install; an existing directory that
// the current user cannot write to is not.
func (e *Ecosystem) Writable(env ecosystem.Environment) bool {
	for _, d := range []string{
		filepath.Join(env.ID, "lib", "node_modules"),
		filepath.Join(env.ID, "bin"),
	} {
		fi, err := os.Stat(d)
		if err != nil || !fi.IsDir() {
			continue
		}
		f, err := os.CreateTemp(d, ".npmitude-write-*")
		if err != nil {
			return false
		}
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
	}
	return true
}

// Resolve batches the intent's operations by op kind (install, upgrade,
// remove). npm has no conflict resolution: it always yields a plan and an
// empty conflict set.
func (e *Ecosystem) Resolve(intent ecosystem.Intent) (ecosystem.Plan, []ecosystem.Conflict, error) {
	var installs, upgrades, removals []ecosystem.Item
	for _, it := range intent.Items {
		switch it.Op {
		case ecosystem.OpInstall:
			installs = append(installs, ecosystem.Item{Name: it.Name, Version: it.Version})
		case ecosystem.OpUpgrade:
			upgrades = append(upgrades, ecosystem.Item{Name: it.Name, Version: it.Version})
		case ecosystem.OpRemove:
			removals = append(removals, ecosystem.Item{Name: it.Name})
		}
	}
	var batches []ecosystem.Batch
	if len(installs) > 0 {
		batches = append(batches, newBatch(ecosystem.OpInstall, installs))
	}
	if len(upgrades) > 0 {
		batches = append(batches, newBatch(ecosystem.OpUpgrade, upgrades))
	}
	if len(removals) > 0 {
		batches = append(batches, newBatch(ecosystem.OpRemove, removals))
	}
	return ecosystem.Plan{Env: intent.Env, Batches: batches}, nil, nil
}

func newBatch(op ecosystem.OpKind, items []ecosystem.Item) ecosystem.Batch {
	args := append([]string{}, opVerbs[op]...)
	for _, it := range items {
		if it.Version != "" {
			args = append(args, it.Name+"@"+it.Version)
			continue
		}
		args = append(args, it.Name)
	}
	return ecosystem.Batch{Op: op, Items: items, Label: "npm " + strings.Join(args, " ")}
}

// Execute runs one plan batch with the environment's own node + npm and
// returns its combined raw output. The output is not interpreted; truth
// comes from a post-run ListInstalled.
func (e *Ecosystem) Execute(ctx context.Context, env ecosystem.Environment, batch ecosystem.Batch) (string, error) {
	node, npmCLI := npmcmd.NodeAndNPM(env.ID)
	args := append([]string{npmCLI}, opVerbs[batch.Op]...)
	for _, it := range batch.Items {
		if it.Version != "" {
			args = append(args, it.Name+"@"+it.Version)
			continue
		}
		args = append(args, it.Name)
	}
	out, err := exec.CommandContext(ctx, node, args...).CombinedOutput()
	return string(out), err
}

type lockHandle struct {
	mgr   *lock.Manager
	envID string
}

func (h *lockHandle) Release() { h.mgr.Release(h.envID) }

// Lock acquires the environment lock. npm has no global lock of its own, so
// this always takes the session-scoped fallback path.
func (e *Ecosystem) Lock(env ecosystem.Environment) (ecosystem.LockHandle, error) {
	if err := e.locks.Acquire(env.ID); err != nil {
		return nil, err
	}
	return &lockHandle{mgr: e.locks, envID: env.ID}, nil
}

// DetectProject reports project applicability. The npm adapter is global-
// scope only in this change; project mode arrives with a later change.
func (e *Ecosystem) DetectProject(root string) (bool, ecosystem.Meta) {
	return false, nil
}

func toEcoDoc(d *registry.Doc) *ecosystem.Doc {
	if d == nil {
		return nil
	}
	return &ecosystem.Doc{
		Name:             d.Name,
		Description:      d.Description,
		Homepage:         d.Homepage,
		Repository:       d.Repository,
		License:          d.License,
		Maintainers:      d.Maintainers,
		Bin:              d.Bin,
		Dependencies:     d.Dependencies,
		PeerDependencies: d.PeerDependencies,
		Versions:         d.Versions,
		Latest:           d.Latest,
		Readme:           d.Readme,
	}
}
