// Package domain holds the npmitude application state model (design D4):
// everything keyed by environment (prefix path), with pure transition helpers.
package domain

import (
	"fmt"
	"sort"
	"strings"

	"npmitude/internal/ecosystem"
)

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

// MarkEntry is one manager's pending mark on a package of a destination.
type MarkEntry struct {
	Mark          Mark
	TargetVersion string // pinned via version screen; "" = latest
}

// PkgState is one package row in a prefix's list. It holds only
// manager-agnostic facts; manager-specific values (e.g. why the tree is
// unhealthy) are supplied by the adapter as metadata. Marks are keyed per
// manager so they survive manager switches and can coexist per destination.
type PkgState struct {
	Name             string
	InstalledVersion string // "" when not installed
	LatestVersion    string // from registry, TTL-cached; "" when unknown
	SizeBytes        *int64 // background-measured; nil until known
	Unhealthy        bool   // generic health flag set by the adapter (e.g. broken deps)
	Automatic        bool   // installed as a dependency, not directly by the user
	Origin           Origin
	Marks            map[string]MarkEntry // keyed by manager id; nil = none
	Description      string
}

// Installed reports whether the package is present in the prefix tree.
func (p *PkgState) Installed() bool { return p.InstalledVersion != "" }

// Upgradable reports whether a newer registry version is known for an
// installed package.
func (p *PkgState) Upgradable() bool {
	return p.Installed() && p.LatestVersion != "" && p.LatestVersion != p.InstalledVersion
}

// MarkFor returns the pending mark manager has on this row (MarkNone if none).
func (p *PkgState) MarkFor(manager string) Mark {
	if p.Marks == nil {
		return MarkNone
	}
	return p.Marks[manager].Mark
}

// TargetVersionFor returns manager's pinned target version ("").
func (p *PkgState) TargetVersionFor(manager string) string {
	if p.Marks == nil {
		return ""
	}
	return p.Marks[manager].TargetVersion
}

// HasMarks reports whether any manager has a pending mark on this row.
func (p *PkgState) HasMarks() bool { return len(p.Marks) > 0 }

// SetMarkFor sets manager's mark with toggle semantics: setting the same mark
// that is already pending clears it back to none.
func (p *PkgState) SetMarkFor(manager string, mk Mark) {
	if p.Marks == nil {
		p.Marks = map[string]MarkEntry{}
	}
	cur := p.Marks[manager]
	if cur.Mark == mk {
		delete(p.Marks, manager)
		return
	}
	entry := MarkEntry{Mark: mk}
	if mk == MarkInstall && cur.TargetVersion != "" {
		entry.TargetVersion = cur.TargetVersion
	}
	p.Marks[manager] = entry
}

// SetMarkEntry force-sets manager's mark (no toggle), replacing any previous
// entry of that manager.
func (p *PkgState) SetMarkEntry(manager string, entry MarkEntry) {
	if p.Marks == nil {
		p.Marks = map[string]MarkEntry{}
	}
	p.Marks[manager] = entry
}

// RevertFor clears manager's pending mark.
func (p *PkgState) RevertFor(manager string) {
	if p.Marks != nil {
		delete(p.Marks, manager)
	}
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

// AutoChar is the flag's auto slot: A when the package was installed
// automatically as a dependency, a space when it was installed directly.
func (p *PkgState) AutoChar() rune {
	if p.Automatic {
		return 'A'
	}
	return ' '
}

// ActionCharFor is manager's pending-action flag character.
func (p *PkgState) ActionCharFor(manager string) rune {
	return actionChar(p.MarkFor(manager))
}

func actionChar(mk Mark) rune {
	switch mk {
	case MarkInstall:
		return '+'
	case MarkRemove:
		return '-'
	case MarkUpgrade:
		return 'u'
	case MarkHold:
		return 'h'
	default:
		return ' '
	}
}

// FlagFor is the three-character <state><auto><action> flag for one manager
// (e.g. "i -", "iA "). Empty slots render as spaces.
func (p *PkgState) FlagFor(manager string) string {
	return string(p.StateChar()) + string(p.AutoChar()) + string(p.ActionCharFor(manager))
}

// chosenResolution records a resolution option the user picked for a
// package's conflict. It stays valid only while the cell's pending marks are
// unchanged (sig), so any later edit to that cell re-opens the conflict.
type chosenResolution struct {
	Label string
	Sig   string
}

// PrefixState is all list state for one environment (one destination). The
// ID is adapter-defined; manager-specific facts about the environment live in
// the adapter, not here. Conflicts holds unresolved conflicts reported by a
// resolver for this destination, keyed by package name; Chosen records the
// user's pick per package until the cell's marks or the disk change.
type PrefixState struct {
	ID         string // environment id, adapter-defined
	Packages   map[string]*PkgState
	Loaded     bool
	SizesKnown bool

	Conflicts map[string][]ecosystem.Conflict
	Chosen    map[string]chosenResolution
}

// CellMarkSig is a stable signature of every pending mark on one package of
// this destination (empty when the cell has no marks).
func (ps *PrefixState) CellMarkSig(name string) string {
	if ps == nil {
		return ""
	}
	p := ps.Packages[name]
	if p == nil || len(p.Marks) == 0 {
		return ""
	}
	mgrs := make([]string, 0, len(p.Marks))
	for mgr := range p.Marks {
		mgrs = append(mgrs, mgr)
	}
	sort.Strings(mgrs)
	var b strings.Builder
	for _, mgr := range mgrs {
		e := p.Marks[mgr]
		fmt.Fprintf(&b, "%s:%d:%s;", mgr, int(e.Mark), e.TargetVersion)
	}
	return b.String()
}

// ResolutionChosen reports whether a valid (still matching the cell's marks)
// resolution pick is recorded for name on this destination.
func (ps *PrefixState) ResolutionChosen(name string) bool {
	if ps == nil {
		return false
	}
	c, ok := ps.Chosen[name]
	return ok && c.Sig == ps.CellMarkSig(name)
}

// UnresolvedConflicts returns the resolver-reported conflicts of one package
// that are not covered by a valid resolution pick.
func (ps *PrefixState) UnresolvedConflicts(name string) []ecosystem.Conflict {
	if ps == nil || len(ps.Conflicts[name]) == 0 {
		return nil
	}
	if ps.ResolutionChosen(name) {
		return nil
	}
	return ps.Conflicts[name]
}

// RecordResolution stores the user's pick for name, bound to the cell's mark
// signature at that moment.
func (ps *PrefixState) RecordResolution(name, label string) {
	if ps == nil {
		return
	}
	if ps.Chosen == nil {
		ps.Chosen = map[string]chosenResolution{}
	}
	ps.Chosen[name] = chosenResolution{Label: label, Sig: ps.CellMarkSig(name)}
}

// ForgetResolution drops any recorded pick for name.
func (ps *PrefixState) ForgetResolution(name string) {
	if ps == nil || ps.Chosen == nil {
		return
	}
	delete(ps.Chosen, name)
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

// SetMark sets manager's mark on a package of a prefix with toggle semantics:
// setting the same mark that is already pending clears it back to none.
func (s *AppState) SetMark(prefixID, name, manager string, mk Mark) {
	ps := s.Prefixes[prefixID]
	if ps == nil {
		return
	}
	if p := ps.Packages[name]; p != nil {
		p.SetMarkFor(manager, mk)
	}
}

// Revert clears manager's mark on a package back to no-action.
func (s *AppState) Revert(prefixID, name, manager string) {
	ps := s.Prefixes[prefixID]
	if ps == nil {
		return
	}
	if p := ps.Packages[name]; p != nil {
		p.RevertFor(manager)
	}
}

// ClearAllMarks clears every pending mark of one manager in a prefix.
func (s *AppState) ClearAllMarks(prefixID, manager string) {
	ps := s.Prefixes[prefixID]
	if ps == nil {
		return
	}
	for _, p := range ps.Packages {
		p.RevertFor(manager)
	}
}

// MarkAllUpgradable marks every upgradable, non-held installed package of a
// prefix for upgrade under manager. Held packages are left untouched.
func (s *AppState) MarkAllUpgradable(prefixID, manager string) int {
	ps := s.Prefixes[prefixID]
	if ps == nil {
		return 0
	}
	n := 0
	for _, p := range ps.Packages {
		if p.Installed() && p.Upgradable() && p.MarkFor(manager) != MarkHold {
			p.SetMarkEntry(manager, MarkEntry{Mark: MarkUpgrade})
			n++
		}
	}
	return n
}

// PendingMarkCount counts rows with a pending mark of one manager in a prefix.
func (s *AppState) PendingMarkCount(prefixID, manager string) int {
	ps := s.Prefixes[prefixID]
	if ps == nil {
		return 0
	}
	n := 0
	for _, p := range ps.Packages {
		if p.MarkFor(manager) != MarkNone {
			n++
		}
	}
	return n
}

// TotalPending counts every pending mark across all prefixes and managers.
func (s *AppState) TotalPending() int {
	n := 0
	for _, ps := range s.Prefixes {
		for _, p := range ps.Packages {
			n += len(p.Marks)
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
