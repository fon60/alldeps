// Package domain holds the npmitude application state model (design D4):
// everything keyed by environment (prefix path), with pure transition helpers.
package domain

import "sort"

type Origin int

const (
	OriginInstalled Origin = iota
	OriginSearch
)

type Mark int

const (
	MarkNone Mark = iota
	MarkInstall
	MarkRemove
	MarkUpgrade
	MarkHold
)

func (m Mark) String() string {
	switch m {
	case MarkInstall:
		return "install"
	case MarkRemove:
		return "remove"
	case MarkUpgrade:
		return "upgrade"
	case MarkHold:
		return "hold"
	default:
		return "none"
	}
}

// PkgState is one package row in a prefix's list. It holds only
// manager-agnostic facts; manager-specific values (e.g. why the tree is
// unhealthy) are supplied by the adapter as metadata.
type PkgState struct {
	Name             string
	InstalledVersion string // "" when not installed
	LatestVersion    string // from registry, TTL-cached; "" when unknown
	SizeBytes        *int64 // background-measured; nil until known
	Unhealthy        bool   // generic health flag set by the adapter (e.g. broken deps)
	Origin           Origin
	Mark             Mark
	TargetVersion    string // pinned via version screen; "" = latest
	Description      string
}

// Installed reports whether the package is present in the prefix tree.
func (p *PkgState) Installed() bool { return p.InstalledVersion != "" }

// Upgradable reports whether a newer registry version is known for an
// installed package.
func (p *PkgState) Upgradable() bool {
	return p.Installed() && p.LatestVersion != "" && p.LatestVersion != p.InstalledVersion
}

// StateChar is the current-state flag character: b unhealthy, i installed,
// p available-not-installed. Unhealthy takes precedence over installed.
func (p *PkgState) StateChar() rune {
	switch {
	case p.Unhealthy:
		return 'b'
	case p.Installed():
		return 'i'
	default:
		return 'p'
	}
}

// ActionChar is the pending-action flag character.
func (p *PkgState) ActionChar() rune {
	switch p.Mark {
	case MarkInstall:
		return '+'
	case MarkRemove:
		return '-'
	case MarkUpgrade:
		return 'u'
	case MarkHold:
		return 'h'
	default:
		return '*'
	}
}

// Flag is the two-character state/action flag (e.g. "i-", "p+").
func (p *PkgState) Flag() string {
	return string(p.StateChar()) + string(p.ActionChar())
}

// PrefixState is all list state for one environment (one destination). The
// ID is adapter-defined; manager-specific facts about the environment live in
// the adapter, not here.
type PrefixState struct {
	ID         string // environment id, adapter-defined
	Packages   map[string]*PkgState
	Loaded     bool
	SizesKnown bool
}

// Pkg returns the package row for name, or nil.
func (ps *PrefixState) Pkg(name string) *PkgState {
	if ps == nil {
		return nil
	}
	return ps.Packages[name]
}

type SortKey int

const (
	SortName SortKey = iota
	SortVersion
	SortSize
	SortState
)

func (k SortKey) String() string {
	switch k {
	case SortName:
		return "name"
	case SortVersion:
		return "version"
	case SortSize:
		return "size"
	case SortState:
		return "state"
	default:
		return "?"
	}
}

// Next returns the next sort key in the cycle.
func (k SortKey) Next() SortKey { return SortKey((int(k) + 1) % 4) }

// AppState is the whole application state machine.
type AppState struct {
	Prefixes       map[string]*PrefixState // keyed by absolute prefix path
	ActivePrefixID string
	FilterText     string // active filter expression; "" = no filter
	SortKey        SortKey
	Applying       bool // single-flight guard for apply runs
}

// NewAppState returns an empty state.
func NewAppState() *AppState {
	return &AppState{
		Prefixes: map[string]*PrefixState{},
		SortKey:  SortName,
	}
}

// Active returns the active prefix's state, or nil.
func (s *AppState) Active() *PrefixState {
	return s.Prefixes[s.ActivePrefixID]
}

// SetMark sets a mark on a package of a prefix with toggle semantics:
// setting the same mark that is already pending clears it back to none.
func (s *AppState) SetMark(prefixID, name string, mk Mark) {
	ps := s.Prefixes[prefixID]
	if ps == nil {
		return
	}
	p := ps.Packages[name]
	if p == nil {
		return
	}
	if p.Mark == mk {
		p.Mark = MarkNone
		p.TargetVersion = ""
		return
	}
	p.Mark = mk
	if mk != MarkInstall {
		p.TargetVersion = ""
	}
}

// Revert clears the mark on a package back to no-action.
func (s *AppState) Revert(prefixID, name string) {
	ps := s.Prefixes[prefixID]
	if ps == nil {
		return
	}
	if p := ps.Packages[name]; p != nil {
		p.Mark = MarkNone
		p.TargetVersion = ""
	}
}

// ClearAllMarks clears every pending mark in a prefix.
func (s *AppState) ClearAllMarks(prefixID string) {
	ps := s.Prefixes[prefixID]
	if ps == nil {
		return
	}
	for _, p := range ps.Packages {
		p.Mark = MarkNone
		p.TargetVersion = ""
	}
}

// MarkAllUpgradable marks every upgradable, non-held installed package for
// upgrade. Held packages are left untouched.
func (s *AppState) MarkAllUpgradable(prefixID string) int {
	ps := s.Prefixes[prefixID]
	if ps == nil {
		return 0
	}
	n := 0
	for _, p := range ps.Packages {
		if p.Installed() && p.Upgradable() && p.Mark != MarkHold {
			p.Mark = MarkUpgrade
			p.TargetVersion = ""
			n++
		}
	}
	return n
}

// PendingMarkCount counts rows with a pending mark in a prefix.
func (s *AppState) PendingMarkCount(prefixID string) int {
	ps := s.Prefixes[prefixID]
	if ps == nil {
		return 0
	}
	n := 0
	for _, p := range ps.Packages {
		if p.Mark != MarkNone {
			n++
		}
	}
	return n
}

// Rows returns the prefix's package rows in a stable order (by name).
func (ps *PrefixState) Rows() []*PkgState {
	rows := make([]*PkgState, 0, len(ps.Packages))
	for _, p := range ps.Packages {
		rows = append(rows, p)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}
