package app

import (
	"context"
	"strings"

	"npmitude/internal/ecosystem"
)

// stubEco is a canned Ecosystem for app tests: the app must be exercisable
// against the port alone, with no npm adapter or node-ecosystem package in
// sight.
type stubEco struct {
	envs      []ecosystem.Environment
	packages  map[string][]ecosystem.Package
	searchFn  func(query string, size, from int) ([]ecosystem.Hit, int, error)
	docs      map[string]*ecosystem.Doc
	readmes   map[string]string
	writable  bool
	latest    map[string]string
	latestErr error
	sizes     map[string]int64
	execOut   string
	execErr   error
	executed  []ecosystem.Batch
}

func newStubEco() *stubEco { return &stubEco{writable: true} }

var stubOpNames = map[ecosystem.OpKind]string{
	ecosystem.OpInstall: "install",
	ecosystem.OpUpgrade: "upgrade",
	ecosystem.OpRemove:  "remove",
}

func (s *stubEco) ID() string { return "stub" }

func (s *stubEco) Discover(ctx context.Context) ([]ecosystem.Environment, error) {
	return s.envs, nil
}

func (s *stubEco) ListInstalled(ctx context.Context, env ecosystem.Environment) ([]ecosystem.Package, error) {
	return s.packages[env.ID], nil
}

func (s *stubEco) Search(ctx context.Context, env ecosystem.Environment, query string, size, from int) ([]ecosystem.Hit, int, error) {
	if s.searchFn != nil {
		return s.searchFn(query, size, from)
	}
	return nil, 0, nil
}

// Resolve batches by op kind with manager-neutral labels; the label format is
// an implementation detail the app only ever displays.
func (s *stubEco) Resolve(intent ecosystem.Intent) (ecosystem.Plan, []ecosystem.Conflict, error) {
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
		var args []string
		for _, it := range items {
			if it.Version != "" {
				args = append(args, it.Name+"@"+it.Version)
				continue
			}
			args = append(args, it.Name)
		}
		batches = append(batches, ecosystem.Batch{Op: op, Items: items, Label: stubOpNames[op] + " " + strings.Join(args, " ")})
	}
	return ecosystem.Plan{Env: intent.Env, Batches: batches}, nil, nil
}

func (s *stubEco) Execute(ctx context.Context, env ecosystem.Environment, batch ecosystem.Batch) (string, error) {
	s.executed = append(s.executed, batch)
	return s.execOut, s.execErr
}

type stubLock struct{}

func (stubLock) Release() {}

func (s *stubEco) Lock(env ecosystem.Environment) (ecosystem.LockHandle, error) {
	return stubLock{}, nil
}

func (s *stubEco) DetectProject(root string) (bool, ecosystem.Meta) { return false, nil }

func (s *stubEco) Capabilities() ecosystem.Caps { return ecosystem.Caps{GlobalScope: true} }

func (s *stubEco) LatestVersions(ctx context.Context, env ecosystem.Environment, names []string) (map[string]string, int, error) {
	if s.latestErr != nil {
		return nil, 0, s.latestErr
	}
	return s.latest, 0, nil
}

func (s *stubEco) UnpackedSize(ctx context.Context, env ecosystem.Environment, name, version string) (int64, error) {
	if b, ok := s.sizes[name]; ok {
		return b, nil
	}
	return 0, nil
}

func (s *stubEco) Doc(ctx context.Context, env ecosystem.Environment, name string, installed bool) (*ecosystem.Doc, bool, error) {
	if d, ok := s.docs[name]; ok {
		return d, installed, nil
	}
	return nil, false, ecosystem.ErrNoRegistry
}

func (s *stubEco) Readme(env ecosystem.Environment, name string) (string, bool) {
	text, ok := s.readmes[name]
	return text, ok
}

func (s *stubEco) Writable(env ecosystem.Environment) bool { return s.writable }
