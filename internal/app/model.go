package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"npmitude/internal/domain"
	"npmitude/internal/ecosystem"
	"npmitude/internal/filter"
	"npmitude/internal/lock"
	"npmitude/internal/sizes"
)

// Version is the application version shown in the header.
var Version = "0.1.0"

// TabKind classifies the persistent context screens that occupy tabs in the
// strip (design D1/D2). The root tab is always a List.
type TabKind int

const (
	TabList TabKind = iota
	TabSearch
	TabInfo
	TabVersions
	TabReadme
	TabResolver
	TabHelp
	TabPlan
)

// Overlay classifies the transient popups that block the active tab without
// appearing in the strip (design D1).
type Overlay int

const (
	OverlayNone Overlay = iota
	OverlayPicker
	OverlayManager
	OverlayTargets
)

// Tab is one open context: its kind, subject identity (Search: query;
// Info/Versions/Readme/Resolver: package name; Help/Plan: none) and all the
// per-kind state, so returning to a tab restores it exactly.
type Tab struct {
	Kind    TabKind
	Subject string

	// List & Search share the list viewport state.
	Cursor  int
	ListTop int

	// Search: results of Subject, loaded in pages.
	Query   string
	Hits    map[string]bool
	Order   []string
	Total   int
	Fetched int
	Loading bool

	// Info.
	Name       string
	Doc        *ecosystem.Doc
	Local      bool
	Err        string
	DestCursor int

	// Versions.
	VerCursor int
	VerTop    int

	// Readme.
	ReadmeLines  []string
	ReadmeScroll int

	// Resolver: the conflicted cell and the cursor over its flat option list.
	RDest   string
	RName   string
	RCursor int

	// Help.
	HelpScroll int

	// Plan: the Yes/No conflict gate popup is up on the plan tab.
	Gate bool
}

type PromptKind int

const (
	PromptFilter PromptKind = iota
	PromptLocal
	PromptSearch
)

// searchPageSize caps one search query to a single bounded page.
const searchPageSize = 20

type promptState struct {
	kind  PromptKind
	input textinput.Model
}

func newPrompt(kind PromptKind) *promptState {
	ti := textinput.New()
	ti.CharLimit = 200
	return &promptState{kind: kind, input: ti}
}

type Model struct {
	state           *domain.AppState
	mode            Mode // global or project scope for this launch
	projectRoot     string
	applicable      []string // project mode: applicable manager ids, stable order
	managers        map[string]ecosystem.Ecosystem
	activeManagerID string
	tabs            []Tab // open contexts; invariant: tabs[0].Kind == TabList, never removed
	tabIdx          int   // index of the active tab
	overlay         Overlay
	width           int
	height          int
	hostname        string
	notice          string
	filterPred      filter.Predicate
	manualOnly      bool // hide automatically installed (A) rows; false = show all
	prompt          *promptState
	pickerCursor    int
	pickerLocked    map[string]bool // env IDs held by another live instance
	activeFallback  string          // last resolved active environment (for vanished-prefix fallback)

	locks *lock.Manager

	envsByManager map[string][]ecosystem.Environment // discovered destinations per manager
	managerCursor int                                // cursor on the manager switcher overlay

	planSizes   map[string]int64 // name -> unpacked size shown in the plan tab
	resolvedSig string           // plan signature the conflict state was last resolved for

	applyBatches  []applyBatch     // queued (destination, manager) batches for the running apply
	applyBatchIdx int
	applyDests    []string          // every destination the running apply touches, in order
	applyLocks    []string          // destinations locked at apply start, released after it
	applyFrom     map[string]string // dest+\x00+name -> installed version at apply start
	applyFailed   int
	applyFromTab  int                // active tab when the run started; dismissed enter returns there
	applyDone     bool               // completion prompt is showing
	applyLog      []string           // raw manager output lines shown on the apply screen
	applyCurrent  string             // command currently running (bottom line while applying)
	applyCancel   context.CancelFunc // aborts the batch currently running
	applyAborted  bool               // user pressed ctrl+c: the whole plan remainder is dropped

	installName         string                  // package the install-target popup marks
	installTargets      []ecosystem.Environment // eligible destinations in the popup
	installTargetCursor int
	installTargetSel    map[string]bool // destination IDs preselected/selected in the popup

	quitConfirm bool // quit confirmation prompt is showing
}

// New builds the app model around the injected ecosystem adapter. The
// adapter's own id becomes the single active manager.
func New(eco ecosystem.Ecosystem) Model {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return Model{
		state:           domain.NewAppState(),
		mode:            ModeGlobal,
		managers:        map[string]ecosystem.Ecosystem{eco.ID(): eco},
		activeManagerID: eco.ID(),
		tabs:            []Tab{{Kind: TabList}},
		envsByManager:   map[string][]ecosystem.Environment{},
		locks:           lock.NewDefault(),
		hostname:        host,
	}
}

// pinNoticeProvider is optionally implemented by managers bound to a project
// to surface a one-shot toolchain notice at startup (e.g. an uninstalled
// .nvmrc pin); the app only displays it, never acts on it.
type pinNoticeProvider interface {
	PinNotice() string
}

// NewProject builds a project-mode model (design D5): one entry per
// applicable manager, each bound to its own adapter instance; exactly the
// first applicable manager starts active. A project with no recognized
// markers still launches, showing a notice instead of guessing. A manager's
// startup notice (e.g. a pin fallback) is surfaced non-blocking on first
// paint.
func NewProject(root string, managers map[string]ecosystem.Ecosystem, applicable []string) Model {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	m := Model{
		state:         domain.NewAppState(),
		mode:          ModeProject,
		projectRoot:   root,
		applicable:    applicable,
		managers:      managers,
		tabs:          []Tab{{Kind: TabList}},
		envsByManager: map[string][]ecosystem.Environment{},
		locks:         lock.NewDefault(),
		hostname:      host,
	}
	if len(applicable) > 0 {
		m.activeManagerID = applicable[0]
		for _, id := range applicable {
			if pn, ok := managers[id].(pinNoticeProvider); ok {
				if n := pn.PinNotice(); n != "" {
					m.notice = n
					break
				}
			}
		}
	} else {
		m.notice = "no recognized package manager for project " + displayPath(root) + " — nothing to manage here"
	}
	return m
}

// isProject reports whether this launch manages a single project directory.
func (m Model) isProject() bool { return m.mode == ModeProject }

// eco returns the adapter of the active manager.
func (m Model) eco() ecosystem.Ecosystem { return m.managers[m.activeManagerID] }

// envs returns the discovered destinations of the active manager.
func (m Model) envs() []ecosystem.Environment { return m.envsByManager[m.activeManagerID] }

// managerIDs returns all known manager ids in stable order.
func (m Model) managerIDs() []string {
	ids := make([]string, 0, len(m.managers))
	for id := range m.managers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// findTab returns the index of the first tab with the given kind and subject,
// or -1. Subject identity (design D3): Search matches by query,
// Info/Versions/Readme/Resolver by package name, Help and Plan are singletons.
func (m Model) findTab(kind TabKind, subject string) int {
	for i, t := range m.tabs {
		if t.Kind == kind && t.Subject == subject {
			return i
		}
	}
	return -1
}

// openTab focuses an existing (kind, subject) tab anywhere in the strip; when
// none exists it appends a new one at the end and activates it. The second
// result reports whether a tab was created.
func (m *Model) openTab(kind TabKind, subject string) (int, bool) {
	if i := m.findTab(kind, subject); i >= 0 {
		m.tabIdx = i
		return i, false
	}
	m.tabs = append(m.tabs, Tab{Kind: kind, Subject: subject})
	m.tabIdx = len(m.tabs) - 1
	return m.tabIdx, true
}

// closeActiveTab removes the active tab (the root List tab is uncloseable) and
// activates its left neighbor. Closing a Search tab discards its unmarked
// result rows; landing on the Plan tab re-arms its conflict gate.
func (m *Model) closeActiveTab() {
	i := m.tabIdx
	if i == 0 {
		return
	}
	if m.tabs[i].Kind == TabSearch {
		m.discardSearchRows(i)
	}
	m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
	m.tabIdx = i - 1
	if m.tabs[m.tabIdx].Kind == TabPlan {
		m.tabs[m.tabIdx].Gate = m.planHasConflicts()
	}
}

// moveTab moves the active tab by delta, inert at the strip edges.
func (m *Model) moveTab(delta int) {
	n := m.tabIdx + delta
	if n < 0 || n >= len(m.tabs) {
		return
	}
	m.tabIdx = n
}

// closeSearchTabs removes every open Search tab, discarding each one's
// unmarked result rows (marks survive in their PrefixState rows).
func (m *Model) closeSearchTabs() {
	for i := len(m.tabs) - 1; i > 0; i-- {
		if m.tabs[i].Kind != TabSearch {
			continue
		}
		m.discardSearchRows(i)
		m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
		if m.tabIdx >= i {
			m.tabIdx--
		}
	}
}

// discardSearchRows removes the unmarked search-origin rows owned by tab idx
// (in its result order) plus any orphaned unmarked search rows no other open
// search tab references. Marked and installed rows are untouched.
func (m *Model) discardSearchRows(idx int) {
	t := m.tabs[idx]
	other := map[string]bool{}
	for i, o := range m.tabs {
		if i == idx || o.Kind != TabSearch {
			continue
		}
		for _, n := range o.Order {
			other[n] = true
		}
	}
	ps := m.state.Active()
	if ps == nil {
		return
	}
	for name, p := range ps.Packages {
		if p.Origin != domain.OriginSearch || p.HasMarks() {
			continue
		}
		owned := false
		for _, n := range t.Order {
			if n == name {
				owned = true
				break
			}
		}
		if owned || !other[name] {
			delete(ps.Packages, name)
		}
	}
}

// activeTab is the currently focused tab.
func (m Model) activeTab() Tab { return m.tabs[m.tabIdx] }

// listTab is the active tab when it is list-like (List or Search), else nil.
func (m Model) listTab() *Tab {
	t := &m.tabs[m.tabIdx]
	if t.Kind == TabList || t.Kind == TabSearch {
		return t
	}
	return nil
}

// hasMoreSearch reports whether the active search tab still has pages to load.
func (m Model) hasMoreSearch() bool {
	t := m.activeTab()
	return t.Kind == TabSearch && !t.Loading && t.Total > 0 && t.Fetched < t.Total
}

// loadMoreSearchCmd fetches the next page of the active search tab's results.
func (m *Model) loadMoreSearchCmd() tea.Cmd {
	ps := m.state.Active()
	if ps == nil || !m.hasMoreSearch() {
		return nil
	}
	t := &m.tabs[m.tabIdx]
	from := t.Fetched
	t.Loading = true
	return m.searchCmd(ps.ID, t.Query, from)
}

// heldNotice renders the refusal message identifying a live lock holder.
func heldNotice(prefixID string, err error) string {
	var held *lock.HeldError
	if errors.As(err, &held) {
		return fmt.Sprintf("environment %s is open in another npmitude (pid %d on %s) — choose a different environment", displayPath(prefixID), held.Holder.PID, held.Holder.Host)
	}
	return "cannot lock environment " + displayPath(prefixID) + ": " + err.Error()
}

// lockedEnvs probes which detected environments are held by another live
// instance (for the picker's locked markers).
func (m Model) lockedEnvs() map[string]bool {
	out := map[string]bool{}
	if m.locks == nil {
		return out
	}
	for _, e := range m.envs() {
		if m.locks.IsHeld(e.ID) {
			out[e.ID] = true
		}
	}
	return out
}

func (m *Model) releaseAll() {
	if m.locks != nil {
		m.locks.ReleaseAll()
	}
}

type discoverMsg struct {
	envsByManager map[string][]ecosystem.Environment
	errs          map[string]error // manager id -> discovery failure
}

// discoverAllCmd asks every known manager for its destinations in the
// background. A failing manager is reported but never blocks first paint.
func (m Model) discoverAllCmd() tea.Cmd {
	ids := m.managerIDs()
	managers := m.managers
	return func() tea.Msg {
		envsByManager := map[string][]ecosystem.Environment{}
		errs := map[string]error{}
		for _, id := range ids {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			envs, err := managers[id].Discover(ctx)
			cancel()
			if err != nil {
				errs[id] = err
				continue
			}
			envsByManager[id] = envs
		}
		return discoverMsg{envsByManager: envsByManager, errs: errs}
	}
}

// refreshCmd re-discovers all managers' environments; the reload of every
// destination happens in a follow-up loadAllEnvsCmd.
func (m Model) refreshCmd() tea.Cmd {
	return m.discoverAllCmd()
}

// loadAllEnvsCmd starts background loads for every not-yet-loaded destination
// of the active manager, so the unified list fills in progressively.
func (m Model) loadAllEnvsCmd() tea.Cmd {
	var cmds []tea.Cmd
	for _, e := range m.envs() {
		if ps := m.state.Prefixes[e.ID]; ps == nil || !ps.Loaded {
			cmds = append(cmds, m.loadEnvCmd(e.ID))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// Messages from async work.

type loadEnvMsg struct {
	prefixID string
	pkgs     map[string]*domain.PkgState
	err      error
}

type outdatedMsg struct {
	prefixID string
	versions map[string]string
	failed   int
	total    int
	err      error
}

type searchMsg struct {
	prefixID string
	query    string
	from     int
	hits     []ecosystem.Hit
	total    int
	err      error
}

// searchCmd queries one page of the environment's package index, starting at
// offset from.
func (m Model) searchCmd(prefixID, query string, from int) tea.Cmd {
	eco := m.eco()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		hits, total, err := eco.Search(ctx, ecosystem.Environment{ID: prefixID}, query, searchPageSize, from)
		return searchMsg{prefixID: prefixID, query: query, from: from, hits: hits, total: total, err: err}
	}
}

// toPkgStates converts adapter-reported installed packages into list rows.
func toPkgStates(pkgs []ecosystem.Package) map[string]*domain.PkgState {
	out := make(map[string]*domain.PkgState, len(pkgs))
	for _, p := range pkgs {
		out[p.Name] = &domain.PkgState{Name: p.Name, InstalledVersion: p.Version, Unhealthy: p.Unhealthy, Automatic: p.Automatic, Origin: domain.OriginInstalled}
	}
	return out
}

// applySearchResults merges one page of search hits into the active prefix's
// list on the Search tab identified by query: installed rows are never
// duplicated or modified (they stay authoritative); previously displayed
// search rows without a pending mark are replaced by the first page, while
// later pages only add; marked search rows are retained regardless. While the
// tab is active, visibleRows shows only its results.
func (m *Model) applySearchResults(prefixID string, query string, from int, hits []ecosystem.Hit, total int) {
	idx := m.findTab(TabSearch, query)
	if idx < 0 {
		return // stale: the search tab was closed or re-queried
	}
	t := &m.tabs[idx]
	ps := m.state.Prefixes[prefixID]
	if ps == nil {
		return
	}
	if from == 0 {
		m.discardSearchRows(idx)
		t.Fetched = 0
		t.Hits = make(map[string]bool, len(hits))
		t.Order = nil
	}
	ordered := make(map[string]bool, len(t.Order))
	for _, n := range t.Order {
		ordered[n] = true
	}
	for _, h := range hits {
		t.Hits[h.Name] = true
		if p, ok := ps.Packages[h.Name]; ok {
			if p.Origin == domain.OriginSearch && !ordered[h.Name] {
				t.Order = append(t.Order, h.Name) // marked leftover at its registry rank
				ordered[h.Name] = true
			}
			continue // installed row stays authoritative; marked search row retained
		}
		ps.Packages[h.Name] = &domain.PkgState{
			Name:          h.Name,
			LatestVersion: h.Version,
			Description:   h.Description,
			Origin:        domain.OriginSearch,
		}
		t.Order = append(t.Order, h.Name)
		ordered[h.Name] = true
	}
	t.Query = query
	t.Fetched += len(hits)
	if total > 0 {
		t.Total = total
	}
	t.Loading = false
}

// checkOutdatedCmd fetches latest versions in the background (never blocks
// first paint).
func (m Model) checkOutdatedCmd(prefixID string, names []string) tea.Cmd {
	eco := m.eco()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		versions, failed, err := eco.LatestVersions(ctx, ecosystem.Environment{ID: prefixID}, names)
		return outdatedMsg{prefixID: prefixID, versions: versions, failed: failed, total: len(names), err: err}
	}
}

func (m Model) Init() tea.Cmd {
	return m.discoverAllCmd()
}

// switchEnv makes id the active environment and loads it if not already
// loaded. The new environment's lock is acquired before the old one is
// released (no gap). Pending marks of other prefixes are untouched (they live
// in their own PrefixState).
func (m *Model) switchEnv(id string) tea.Cmd {
	if id == "" || id == m.state.ActivePrefixID {
		return nil
	}
	if m.locks != nil {
		if err := m.locks.Acquire(id); err != nil {
			m.notice = heldNotice(id, err)
			return nil
		}
	}
	old := m.state.ActivePrefixID
	m.state.ActivePrefixID = id
	if m.locks != nil && old != "" {
		m.locks.Release(old)
	}
	if _, ok := m.state.Prefixes[id]; !ok {
		m.state.Prefixes[id] = &domain.PrefixState{ID: id, Packages: map[string]*domain.PkgState{}}
	}
	m.tabs[0].Cursor = 0
	m.tabs[0].ListTop = 0
	m.notice = ""
	m.closeSearchTabs() // results referenced the previous environment's rows
	ps := m.state.Prefixes[id]
	if ps.Loaded {
		return nil
	}
	return m.loadEnvCmd(id)
}

// dropVanishedEnvs removes environments that disappeared since the last scan
// and, if the selected one vanished, falls back to the manager's active
// environment with a notice (spec: Vanished prefix handling). It returns a
// load command for the fallback when one must be loaded.
func (m *Model) dropVanishedEnvs() tea.Cmd {
	if len(m.envs()) == 0 {
		if m.state.ActivePrefixID != "" {
			m.notice = "no environments detected for manager " + m.activeManagerID
		}
		return nil
	}
	live := map[string]bool{}
	for _, e := range m.envs() {
		live[e.ID] = true
	}
	active := m.state.ActivePrefixID
	if active != "" && !live[active] {
		fallback := m.activeFallback
		if fallback == "" || !live[fallback] {
			fallback = m.envs()[0].ID
		}
		cmd := m.switchEnv(fallback)
		if m.state.ActivePrefixID == fallback {
			m.notice = "prefix " + displayPath(active) + " no longer exists — fell back to " + displayPath(fallback)
		}
		return cmd
	}
	return nil
}

// cycleEnv switches to the next detected environment in version order.
func (m *Model) cycleEnv() tea.Cmd {
	if len(m.envs()) == 0 {
		return nil
	}
	idx := -1
	for i, e := range m.envs() {
		if e.ID == m.state.ActivePrefixID {
			idx = i
			break
		}
	}
	next := m.envs()[(idx+1)%len(m.envs())]
	return m.switchEnv(next.ID)
}

type planSizeMsg struct {
	name  string
	bytes int64
	err   error
}

// planSizeCmd fetches the approximate unpacked size of one install target for
// the plan screen. Failures leave the row showing "…".
func (m Model) planSizeCmd(prefixID, name, version string) tea.Cmd {
	eco := m.eco()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		b, err := eco.UnpackedSize(ctx, ecosystem.Environment{ID: prefixID}, name, version)
		return planSizeMsg{name: name, bytes: b, err: err}
	}
}

type infoDataMsg struct {
	name  string
	doc   *ecosystem.Doc
	local bool
	err   error
}

// infoCmd loads package metadata for the info screen: the local document for
// installed packages (offline-capable), otherwise the registry document.
func (m Model) infoCmd(prefixID, name string, installed bool) tea.Cmd {
	eco := m.eco()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		doc, local, err := eco.Doc(ctx, ecosystem.Environment{ID: prefixID}, name, installed)
		return infoDataMsg{name: name, doc: doc, local: local, err: err}
	}
}

type versionsMsg struct {
	name string
	doc  *ecosystem.Doc
	err  error
}

// versionsCmd fetches the full package document for the version history view.
func (m Model) versionsCmd(prefixID, name string) tea.Cmd {
	eco := m.eco()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		doc, _, err := eco.Doc(ctx, ecosystem.Environment{ID: prefixID}, name, false)
		if err != nil {
			return versionsMsg{name: name, err: err}
		}
		domain.SortVersions(doc.Versions)
		return versionsMsg{name: name, doc: doc}
	}
}

type readmeMsg struct {
	name  string
	text  string
	found bool
	err   error
}

// readmeFetchCmd fetches the package document on demand for its readme field.
func (m Model) readmeFetchCmd(prefixID, name string) tea.Cmd {
	eco := m.eco()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		doc, _, err := eco.Doc(ctx, ecosystem.Environment{ID: prefixID}, name, false)
		if err != nil {
			return readmeMsg{name: name, err: err}
		}
		return readmeMsg{name: name, text: doc.Readme, found: doc.Readme != ""}
	}
}

type applyBatchMsg struct {
	idx     int
	cmdLine string // human-readable command line supplied by the adapter
	output  string // combined stdout+stderr of the manager invocation
	err     error
}

type applyReloadMsg struct {
	pkgsByDest map[string]map[string]*domain.PkgState // destination -> fresh installed state
	errs       map[string]error                       // destination -> re-read failure
}

// targetVersionFor is the version a mark will apply under manager: a pinned
// version if set, else the known latest.
func targetVersionFor(p *domain.PkgState, manager string) string {
	if v := p.TargetVersionFor(manager); v != "" {
		return v
	}
	return p.LatestVersion
}

// hasInstallMark reports whether any manager has an install mark pending.
func hasInstallMark(p *domain.PkgState) bool {
	for _, e := range p.Marks {
		if e.Mark == domain.MarkInstall {
			return true
		}
	}
	return false
}

// liveDests returns every destination the plan may touch, sorted. In global
// mode that is every destination of every manager; in project mode only the
// active adapter's destinations (per-adapter isolation).
func (m Model) liveDests() []string {
	seen := map[string]bool{}
	var out []string
	ids := m.managerIDs()
	if m.isProject() {
		ids = []string{m.activeManagerID}
	}
	for _, id := range ids {
		for _, e := range m.envsByManager[id] {
			if !seen[e.ID] {
				seen[e.ID] = true
				out = append(out, e.ID)
			}
		}
	}
	sort.Strings(out)
	return out
}

type planGroup struct {
	dest     string
	manager  string
	installs []*domain.PkgState
	removals []*domain.PkgState
	upgrades []*domain.PkgState
	invalid  bool // destination marked under two managers: skipped, not executed
}

// planOps collects every executable pending mark as an op (holds excluded; in
// project mode only the active adapter's marks participate, per-adapter
// isolation). The order is stable: live destinations, then row order.
func (m Model) planOps() []domain.Op {
	var ops []domain.Op
	for _, dest := range m.liveDests() {
		ps := m.state.Prefixes[dest]
		if ps == nil {
			continue
		}
		for _, p := range ps.Rows() {
			for mgr, entry := range p.Marks {
				if entry.Mark == domain.MarkHold || entry.Mark == domain.MarkNone {
					continue
				}
				if m.isProject() && mgr != m.activeManagerID {
					continue
				}
				ops = append(ops, domain.Op{Destination: dest, Manager: mgr, Name: p.Name, Mark: entry.Mark, Version: entry.TargetVersion})
			}
		}
	}
	return ops
}

// planSignature is a stable string over the executable pending marks; it
// changes exactly when the set of operations to resolve changes.
func (m Model) planSignature() string {
	ops := append([]domain.Op(nil), m.planOps()...)
	sort.Slice(ops, func(i, j int) bool {
		a, b := ops[i], ops[j]
		if a.Destination != b.Destination {
			return a.Destination < b.Destination
		}
		if a.Manager != b.Manager {
			return a.Manager < b.Manager
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if int(a.Mark) != int(b.Mark) {
			return int(a.Mark) < int(b.Mark)
		}
		return a.Version < b.Version
	})
	var b strings.Builder
	for _, op := range ops {
		fmt.Fprintf(&b, "%s\x00%s\x00%s\x00%d\x00%s;", op.Destination, op.Manager, op.Name, int(op.Mark), op.Version)
	}
	return b.String()
}

// planGroups derives the ordered (destination, manager) groups of all pending
// marks across every live destination and manager. A group whose destination
// carries marks of two different managers is flagged invalid (spec:
// one-manager-per-destination guard).
func (m Model) planGroups() []planGroup {
	plan := &domain.Plan{Ops: m.planOps()}
	invalid := map[string]bool{}
	for _, d := range plan.InvalidDestinations() {
		invalid[d] = true
	}
	var groups []planGroup
	for _, g := range plan.Groups() {
		ps := m.state.Prefixes[g.Destination]
		pg := planGroup{dest: g.Destination, manager: g.Manager, invalid: invalid[g.Destination]}
		for _, op := range g.Ops {
			if ps == nil {
				continue
			}
			p := ps.Packages[op.Name]
			if p == nil {
				continue
			}
			switch op.Mark {
			case domain.MarkInstall:
				pg.installs = append(pg.installs, p)
			case domain.MarkRemove:
				pg.removals = append(pg.removals, p)
			case domain.MarkUpgrade:
				pg.upgrades = append(pg.upgrades, p)
			}
		}
		groups = append(groups, pg)
	}
	return groups
}

// pendingOps counts every executable pending mark (holds excluded).
func (m Model) pendingOps() int {
	n := 0
	for _, g := range m.planGroups() {
		n += len(g.installs) + len(g.removals) + len(g.upgrades)
	}
	return n
}

type resolveMsg struct {
	dest      string
	manager   string
	conflicts []ecosystem.Conflict
	err       error
}

// resolveOneCmd asks one manager to resolve the pending operations of one
// destination in the background; its reported conflicts are stored per
// (destination, package). Failures are non-fatal: the previous state stands.
func resolveOneCmd(eco ecosystem.Ecosystem, dest, manager string, items []ecosystem.MarkedItem) tea.Cmd {
	return func() tea.Msg {
		_, conflicts, err := eco.Resolve(ecosystem.Intent{Env: ecosystem.Environment{ID: dest}, Items: items})
		if err != nil {
			return resolveMsg{dest: dest, manager: manager, err: err}
		}
		return resolveMsg{dest: dest, manager: manager, conflicts: conflicts}
	}
}

// resolveGroupsCmd starts a background resolve for every valid
// (destination, manager) group that has pending operations.
func (m Model) resolveGroupsCmd() tea.Cmd {
	type job struct {
		eco     ecosystem.Ecosystem
		dest    string
		manager string
		items   []ecosystem.MarkedItem
	}
	var jobs []job
	seen := map[string]bool{}
	for _, g := range m.planGroups() {
		if g.invalid {
			continue
		}
		key := g.dest + "\x00" + g.manager
		if seen[key] {
			continue
		}
		seen[key] = true
		eco := m.managers[g.manager]
		if eco == nil {
			continue
		}
		items := make([]ecosystem.MarkedItem, 0, len(g.installs)+len(g.upgrades)+len(g.removals))
		for _, p := range g.installs {
			items = append(items, ecosystem.MarkedItem{Op: ecosystem.OpInstall, Name: p.Name, Version: targetVersionFor(p, g.manager)})
		}
		for _, p := range g.upgrades {
			items = append(items, ecosystem.MarkedItem{Op: ecosystem.OpUpgrade, Name: p.Name, Version: targetVersionFor(p, g.manager)})
		}
		for _, p := range g.removals {
			items = append(items, ecosystem.MarkedItem{Op: ecosystem.OpRemove, Name: p.Name})
		}
		jobs = append(jobs, job{eco: eco, dest: g.dest, manager: g.manager, items: items})
	}
	if len(jobs) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(jobs))
	for _, j := range jobs {
		cmds = append(cmds, resolveOneCmd(j.eco, j.dest, j.manager, j.items))
	}
	return tea.Batch(cmds...)
}

// maybeResolveConflicts refreshes the conflict state when the pending-mark
// set changed since the last resolve. It never blocks: the work runs as a
// background command (consistent with the other enrichment passes).
func (m *Model) maybeResolveConflicts() tea.Cmd {
	sig := m.planSignature()
	if sig == m.resolvedSig {
		return nil
	}
	m.resolvedSig = sig
	if sig == "" {
		for _, ps := range m.state.Prefixes {
			ps.Conflicts = nil
			ps.Chosen = nil
		}
		return nil
	}
	return m.resolveGroupsCmd()
}

// withConflictRefresh appends the background re-resolution command (when the
// mark set changed) to a keypress's returned commands.
func (m Model) withConflictRefresh(cmds ...tea.Cmd) (Model, tea.Cmd) {
	if c := m.maybeResolveConflicts(); c != nil {
		cmds = append(cmds, c)
	}
	switch len(cmds) {
	case 0:
		return m, nil
	case 1:
		return m, cmds[0]
	default:
		return m, tea.Batch(cmds...)
	}
}

// dedupeCopy is one installed copy of a package on one destination.
type dedupeCopy struct {
	dest    string
	version string
	size    *int64
}

// dedupeCopies lists the installed copies of name across the loaded
// destinations of manager (best-ranked first) when the manager advertises
// HasDedupe; otherwise nil.
func (m Model) dedupeCopies(managerID, name string) []dedupeCopy {
	eco := m.managers[managerID]
	if eco == nil || !eco.Capabilities().HasDedupe {
		return nil
	}
	rankOf := map[string]string{}
	for _, e := range m.envsByManager[managerID] {
		rankOf[e.ID] = e.Rank
	}
	var out []dedupeCopy
	for _, e := range m.envsByManager[managerID] {
		ps := m.state.Prefixes[e.ID]
		if ps == nil || !ps.Loaded {
			continue
		}
		p := ps.Packages[name]
		if p == nil || !p.Installed() {
			continue
		}
		out = append(out, dedupeCopy{dest: e.ID, version: p.InstalledVersion, size: p.SizeBytes})
	}
	sort.SliceStable(out, func(i, j int) bool {
		r1, r2 := rankOf[out[i].dest], rankOf[out[j].dest]
		switch {
		case r1 == r2:
			return out[i].dest < out[j].dest
		case r1 == "":
			return false
		case r2 == "":
			return true
		default:
			return domain.CompareVersions(r1, r2) > 0
		}
	})
	return out
}

// sumKnownSizes adds the measured sizes of copies; ok is false when any copy
// has no measurement yet.
func sumKnownSizes(copies []dedupeCopy) (int64, bool) {
	total := int64(0)
	for _, c := range copies {
		if c.size == nil {
			return 0, false
		}
		total += *c.size
	}
	return total, true
}

func sizeDeltaPtr(copies []dedupeCopy) *int64 {
	b, ok := sumKnownSizes(copies)
	if !ok {
		return nil
	}
	d := -b
	return &d
}

// buildDedupeConflict turns redundant copies of one package into a conflict
// with align/consolidate/remove options. The align option keeps the copy at
// the highest version (best-ranked destination on ties) and removes the rest;
// each consolidate option keeps exactly one named destination's copy; remove
// drops every copy.
func buildDedupeConflict(name string, copies []dedupeCopy) ecosystem.Conflict {
	bestIdx := 0
	for i, c := range copies {
		if domain.CompareVersions(c.version, copies[bestIdx].version) > 0 {
			bestIdx = i
		}
	}
	best := copies[bestIdx]
	var parts []string
	for _, c := range copies {
		parts = append(parts, fmt.Sprintf("%s on %s", c.version, displayPath(c.dest)))
	}
	msg := fmt.Sprintf("redundant copies of %s: %s", name, strings.Join(parts, ", "))

	var options []ecosystem.ResolutionOption
	others := make([]dedupeCopy, 0, len(copies)-1)
	for _, c := range copies {
		if c.dest != best.dest {
			others = append(others, c)
		}
	}
	options = append(options, ecosystem.ResolutionOption{
		Label:       fmt.Sprintf("Align to %s (keep on %s)", best.version, displayPath(best.dest)),
		Description: fmt.Sprintf("remove the other %d copy(ies); the %s copy stays", len(others), best.version),
		SizeDelta:   sizeDeltaPtr(others),
		Effect:      ecosystem.ResolutionEffect{Kind: "remove", Name: name, Destinations: destIDs(others)},
	})
	for _, c := range copies {
		if c.dest == best.dest {
			continue
		}
		rest := make([]dedupeCopy, 0, len(copies)-1)
		for _, o := range copies {
			if o.dest != c.dest {
				rest = append(rest, o)
			}
		}
		options = append(options, ecosystem.ResolutionOption{
			Label:       fmt.Sprintf("Consolidate to %s (%s)", displayPath(c.dest), c.version),
			Description: fmt.Sprintf("keep only the copy on %s; remove the other %d copy(ies)", displayPath(c.dest), len(rest)),
			SizeDelta:   sizeDeltaPtr(rest),
			Effect:      ecosystem.ResolutionEffect{Kind: "remove", Name: name, Destinations: destIDs(rest)},
		})
	}
	options = append(options, ecosystem.ResolutionOption{
		Label:       "Remove all copies",
		Description: fmt.Sprintf("remove %s from all %d destination(s)", name, len(copies)),
		SizeDelta:   sizeDeltaPtr(copies),
		Effect:      ecosystem.ResolutionEffect{Kind: "remove", Name: name, Destinations: destIDs(copies)},
	})

	return ecosystem.Conflict{Package: name, Message: msg, Options: options}
}

func destIDs(copies []dedupeCopy) []string {
	out := make([]string, 0, len(copies))
	for _, c := range copies {
		out = append(out, c.dest)
	}
	return out
}

// dedupeConflictFor derives the redundancy conflict for (dest, name) under
// one manager: non-nil when dest holds an installed copy and at least one
// other loaded destination of the manager does too.
func (m Model) dedupeConflictFor(managerID, dest, name string) *ecosystem.Conflict {
	copies := m.dedupeCopies(managerID, name)
	if len(copies) < 2 {
		return nil
	}
	for _, c := range copies {
		if c.dest == dest {
			cf := buildDedupeConflict(name, copies)
			return &cf
		}
	}
	return nil
}

// cellConflicted reports whether (dest, name) is involved in an unresolved
// conflict: one the resolver reported for that destination, or a derived
// cross-destination redundancy under any manager owning it. A valid recorded
// resolution pick suppresses the cell until the cell's marks change.
func (m Model) cellConflicted(dest, name string) bool {
	ps := m.state.Prefixes[dest]
	stored := false
	if ps != nil && len(ps.UnresolvedConflicts(name)) > 0 {
		stored = true
	}
	derived := false
	for _, id := range m.managerIDs() {
		if m.dedupeConflictFor(id, dest, name) != nil {
			derived = true
			break
		}
	}
	if !stored && !derived {
		return false
	}
	if ps != nil && ps.ResolutionChosen(name) {
		return false
	}
	return true
}

// conflictsForCell merges the resolver-reported and derived conflicts of one
// cell (chosen picks included, so a just-resolved cell can still show what it
// was about).
func (m Model) conflictsForCell(dest, name string) []ecosystem.Conflict {
	var out []ecosystem.Conflict
	if ps := m.state.Prefixes[dest]; ps != nil {
		out = append(out, ps.Conflicts[name]...)
	}
	for _, id := range m.managerIDs() {
		if cf := m.dedupeConflictFor(id, dest, name); cf != nil {
			out = append(out, *cf)
		}
	}
	return out
}

// rowConflicted reports whether any destination of the active manager carries
// an unresolved conflict for the aggregated row.
func (m Model) rowConflicted(u domain.UnifiedRow) bool {
	for _, e := range m.envs() {
		if m.cellConflicted(e.ID, u.Name) {
			return true
		}
	}
	return false
}

// conflictDestFor returns the first destination of the active manager with an
// unresolved conflict for name ("" when none).
func (m Model) conflictDestFor(name string) string {
	for _, e := range m.envs() {
		if m.cellConflicted(e.ID, name) {
			return e.ID
		}
	}
	return ""
}

// nextConflictCell returns the first plan op cell with an unresolved
// conflict, in plan order ("" when none).
func (m Model) nextConflictCell() (string, string) {
	for _, g := range m.planGroups() {
		if g.invalid {
			continue
		}
		for _, list := range [][]*domain.PkgState{g.installs, g.upgrades, g.removals} {
			for _, p := range list {
				if m.cellConflicted(g.dest, p.Name) {
					return g.dest, p.Name
				}
			}
		}
	}
	return "", ""
}

// planHasConflicts reports whether any op of the current plan touches an
// unresolved conflict cell.
func (m Model) planHasConflicts() bool {
	dest, _ := m.nextConflictCell()
	return dest != ""
}

// managerForDest returns the manager id that owns dest (active manager as
// fallback).
func (m Model) managerForDest(dest string) string {
	for _, id := range m.managerIDs() {
		for _, e := range m.envsByManager[id] {
			if e.ID == dest {
				return id
			}
		}
	}
	return m.activeManagerID
}

// openResolver opens (or focuses) the resolver tab for one conflicted cell.
func (m Model) openResolver(dest, name string) (Model, tea.Cmd) {
	idx, _ := m.openTab(TabResolver, name)
	t := &m.tabs[idx]
	t.RDest = dest
	t.RName = name
	t.RCursor = 0
	return m, nil
}

// resolverOptionRows is the flat option list of a resolver tab: one entry per
// option of each conflict of the cell.
func (m Model) resolverOptionRows(t *Tab) []ecosystem.ResolutionOption {
	var out []ecosystem.ResolutionOption
	for _, cf := range m.conflictsForCell(t.RDest, t.RName) {
		out = append(out, cf.Options...)
	}
	return out
}

// applyResolutionOption applies the chosen option's effect to the marks and
// records the pick on every cell involved in the conflict.
func (m *Model) applyResolutionOption(opt ecosystem.ResolutionOption) {
	var rdest, rname string
	if m.tabs[m.tabIdx].Kind == TabResolver {
		rdest = m.tabs[m.tabIdx].RDest
		rname = m.tabs[m.tabIdx].RName
	}
	name := opt.Effect.Name
	if name == "" {
		name = rname
	}
	dests := opt.Effect.Destinations
	if len(dests) == 0 {
		dests = []string{rdest}
	}
	mgrOf := func(d string) string { return m.managerForDest(d) }
	switch opt.Effect.Kind {
	case "install":
		for _, d := range dests {
			m.setInstallMarkForDest(d, name)
			if ps := m.state.Prefixes[d]; ps != nil {
				if p := ps.Packages[name]; p != nil {
					p.SetMarkEntry(mgrOf(d), domain.MarkEntry{Mark: domain.MarkInstall, TargetVersion: opt.Effect.TargetVersion})
				}
			}
		}
	case "upgrade":
		for _, d := range dests {
			if ps := m.state.Prefixes[d]; ps != nil {
				if p := ps.Packages[name]; p != nil {
					p.SetMarkEntry(mgrOf(d), domain.MarkEntry{Mark: domain.MarkUpgrade, TargetVersion: opt.Effect.TargetVersion})
				}
			}
		}
	case "remove":
		for _, d := range dests {
			if ps := m.state.Prefixes[d]; ps != nil {
				if p := ps.Packages[name]; p != nil {
					p.SetMarkEntry(mgrOf(d), domain.MarkEntry{Mark: domain.MarkRemove})
				}
			}
		}
	case "skip":
		for _, d := range dests {
			m.state.Revert(d, name, mgrOf(d))
		}
	}
	for _, ps := range m.state.Prefixes {
		if len(ps.Conflicts[name]) > 0 || m.cellConflicted(ps.ID, name) {
			ps.RecordResolution(name, opt.Label)
		}
	}
	m.notice = fmt.Sprintf("%s: %s", name, opt.Label)
	m.afterResolutionChoice()
}

// afterResolutionChoice keeps the resolver tab open while the same cell is
// still conflicted and otherwise closes it (the left neighbor — re-arming the
// plan gate when it is the plan — becomes active).
func (m *Model) afterResolutionChoice() {
	if m.tabs[m.tabIdx].Kind != TabResolver {
		return
	}
	t := &m.tabs[m.tabIdx]
	if m.cellConflicted(t.RDest, t.RName) {
		t.RCursor = 0
		return
	}
	m.closeActiveTab()
}

// openPlan opens (or focuses) the plan tab and starts fetching install sizes.
// When a pending operation touches an unresolved conflict, the Yes/No gate
// popup is raised on top of the plan (spec: plan gate). The plan renders live
// from current marks whenever it is shown.
func (m Model) openPlan() (Model, tea.Cmd) {
	idx, _ := m.openTab(TabPlan, "")
	m.tabs[idx].Gate = m.planHasConflicts()
	m.planSizes = map[string]int64{}
	var cmds []tea.Cmd
	for _, g := range m.planGroups() {
		for _, p := range g.installs {
			cmds = append(cmds, m.planSizeCmd(g.dest, p.Name, targetVersionFor(p, g.manager)))
		}
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

// openInfo opens (or focuses) the info tab for the selected row and starts
// loading its metadata on a freshly created tab (local package.json when
// installed, registry document otherwise). The per-destination table cursor
// starts on the headline destination. An existing tab keeps its loaded state.
func (m Model) openInfo() (Model, tea.Cmd) {
	u := m.selectedUnified()
	if u == nil {
		return m, nil
	}
	idx, created := m.openTab(TabInfo, u.Name)
	if !created {
		return m, nil
	}
	t := &m.tabs[idx]
	t.Name = u.Name
	envID := m.state.ActivePrefixID
	if u.HeadlineID != "" {
		envID = u.HeadlineID
	}
	for i, e := range m.envs() {
		if e.ID == envID {
			t.DestCursor = i
			break
		}
	}
	return m, m.infoCmd(envID, u.Name, u.Installed())
}

// openVersions opens (or focuses) the version-history tab for the info
// package; when the info doc carries no version list (local doc of an
// installed package) it fetches the registry document first.
func (m Model) openVersions() (Model, tea.Cmd) {
	it := m.activeTab()
	idx, created := m.openTab(TabVersions, it.Name)
	if !created {
		return m, nil
	}
	t := &m.tabs[idx]
	t.Name = it.Name
	if it.Doc != nil && len(it.Doc.Versions) > 0 {
		t.Doc = it.Doc
		domain.SortVersions(t.Doc.Versions) // newest first regardless of source order (design D4)
		m.positionVersionCursor(idx)
		return m, nil
	}
	return m, m.versionsCmd(m.state.ActivePrefixID, it.Name)
}

// openVersionsByName opens (or focuses) the version-history tab for name from
// a list-like tab and starts fetching its document on a freshly created tab;
// an existing tab is only focused, never duplicated or re-fetched.
func (m Model) openVersionsByName(name string) (Model, tea.Cmd) {
	idx, created := m.openTab(TabVersions, name)
	if !created {
		return m, nil
	}
	t := &m.tabs[idx]
	t.Name = name
	return m, m.versionsCmd(m.state.ActivePrefixID, name)
}

// positionVersionCursor puts the cursor of a versions tab on the installed
// version, else the latest, else the first entry.
func (m *Model) positionVersionCursor(idx int) {
	t := &m.tabs[idx]
	if t.Doc == nil || len(t.Doc.Versions) == 0 {
		t.VerCursor = 0
		return
	}
	prefer := ""
	if ps := m.state.Active(); ps != nil {
		if p := ps.Packages[t.Name]; p != nil && p.InstalledVersion != "" {
			prefer = p.InstalledVersion
		}
	}
	if prefer == "" {
		prefer = t.Doc.Latest
	}
	for i, v := range t.Doc.Versions {
		if v == prefer {
			t.VerCursor = i
			break
		}
	}
	m.syncVersionTop(idx)
}

// syncVersionTop keeps the versions cursor inside the visible viewport: the
// list scrolls only when the cursor leaves the currently displayed range, in
// either direction. Visible data rows exclude the title line and the column
// header (the corrected package-list math, design D3).
func (m *Model) syncVersionTop(idx int) {
	t := &m.tabs[idx]
	h := m.versionContentH()
	n := 0
	if t.Doc != nil {
		n = len(t.Doc.Versions)
	}
	maxTop := n - h
	if maxTop < 0 {
		maxTop = 0
	}
	if t.VerCursor < t.VerTop {
		t.VerTop = t.VerCursor
	}
	if t.VerCursor >= t.VerTop+h {
		t.VerTop = t.VerCursor - h + 1
	}
	if t.VerTop > maxTop {
		t.VerTop = maxTop
	}
	if t.VerTop < 0 {
		t.VerTop = 0
	}
}

// pinVersion marks the versions tab's package for install/upgrade at exactly
// the selected version (spec: version history) and returns to the info tab of
// the same package when one is open.
func (m *Model) pinVersion() {
	t := &m.tabs[m.tabIdx]
	if t.Doc == nil || t.VerCursor >= len(t.Doc.Versions) {
		return
	}
	v := t.Doc.Versions[t.VerCursor]
	ps := m.state.Active()
	if ps == nil {
		return
	}
	p := ps.Packages[t.Name]
	if p == nil {
		return
	}
	if p.Installed() {
		p.SetMarkEntry(m.activeManagerID, domain.MarkEntry{Mark: domain.MarkUpgrade, TargetVersion: v})
		m.notice = fmt.Sprintf("%s marked for upgrade to %s", p.Name, v)
	} else {
		p.SetMarkEntry(m.activeManagerID, domain.MarkEntry{Mark: domain.MarkInstall, TargetVersion: v})
		m.notice = fmt.Sprintf("%s marked for install at %s", p.Name, v)
	}
	if ii := m.findTab(TabInfo, t.Name); ii >= 0 {
		m.tabIdx = ii
	}
}

// openReadme opens (or focuses) the README tab. Source priority (design D9):
// local README file for installed packages, the already-loaded registry
// readme, then an on-demand registry fetch; absent → a notice line.
func (m Model) openReadme() (Model, tea.Cmd) {
	it := m.activeTab()
	idx, created := m.openTab(TabReadme, it.Name)
	if !created {
		return m, nil
	}
	t := &m.tabs[idx]
	t.Name = it.Name
	var text string
	found := false
	if ps := m.state.Active(); ps != nil {
		if p := ps.Packages[it.Name]; p != nil && p.Installed() {
			text, found = m.eco().Readme(ecosystem.Environment{ID: m.state.ActivePrefixID}, it.Name)
		}
	}
	if !found && it.Doc != nil && it.Doc.Readme != "" {
		text, found = it.Doc.Readme, true
	}
	if found {
		m.setReadmeLines(idx, text)
		return m, nil
	}
	t.ReadmeLines = []string{"loading…"}
	return m, m.readmeFetchCmd(m.state.ActivePrefixID, it.Name)
}

func (m *Model) setReadmeLines(idx int, text string) {
	t := &m.tabs[idx]
	if text == "" {
		t.ReadmeLines = []string{"no README available"}
	} else {
		t.ReadmeLines = markdownToText(text)
	}
	t.ReadmeScroll = 0
}

// applyBatch is one queued invocation bound to its destination and the
// manager that executes it.
type applyBatch struct {
	dest    string
	manager string
	batch   ecosystem.Batch
}

func (ab applyBatch) cmdLine() string { return ab.batch.Label + " @ " + displayPath(ab.dest) }

// startApply begins the single-flight apply run over every valid
// (destination, manager) group: each group is resolved through its own
// manager, locks are taken for all touched destinations before any batch
// runs, versions are snapshotted for post-apply reconciliation, and the
// first batch launches. Invalid groups (one-manager-per-destination
// violations) and operations involved in unresolved conflicts are skipped
// and reported in the apply log; conflicting marks stay pending for a retry.
func (m Model) startApply() (Model, tea.Cmd) {
	if m.state.Applying {
		m.notice = "apply already in progress"
		return m, nil
	}
	var valid []planGroup
	for _, g := range m.planGroups() {
		if !g.invalid {
			valid = append(valid, g)
		}
	}
	if len(valid) == 0 {
		m.notice = "no executable operations — every group is invalid (a destination is marked under two managers)"
		return m, nil
	}
	for _, g := range valid {
		eco := m.managers[g.manager]
		if eco == nil || !eco.Writable(ecosystem.Environment{ID: g.dest}) {
			m.notice = fmt.Sprintf("destination %s is not writable under %s — make it writable or drop its marks before applying", displayPath(g.dest), g.manager)
			return m, nil
		}
	}
	var batches []applyBatch
	dests := []string{}
	var conflictSkips []string
	for _, g := range valid {
		eco := m.managers[g.manager]
		items := make([]ecosystem.MarkedItem, 0, len(g.installs)+len(g.upgrades)+len(g.removals))
		var skipped []string
		for _, p := range g.installs {
			if m.cellConflicted(g.dest, p.Name) {
				skipped = append(skipped, "install "+p.Name)
				continue
			}
			items = append(items, ecosystem.MarkedItem{Op: ecosystem.OpInstall, Name: p.Name, Version: targetVersionFor(p, g.manager)})
		}
		for _, p := range g.upgrades {
			if m.cellConflicted(g.dest, p.Name) {
				skipped = append(skipped, "upgrade "+p.Name)
				continue
			}
			items = append(items, ecosystem.MarkedItem{Op: ecosystem.OpUpgrade, Name: p.Name, Version: targetVersionFor(p, g.manager)})
		}
		for _, p := range g.removals {
			if m.cellConflicted(g.dest, p.Name) {
				skipped = append(skipped, "remove "+p.Name)
				continue
			}
			items = append(items, ecosystem.MarkedItem{Op: ecosystem.OpRemove, Name: p.Name})
		}
		for _, s := range skipped {
			conflictSkips = append(conflictSkips, fmt.Sprintf("skipped %s on %s — unresolved conflict; resolve it and re-apply", s, displayPath(g.dest)))
		}
		if len(items) == 0 {
			continue
		}
		dests = append(dests, g.dest)
		plan, _, err := eco.Resolve(ecosystem.Intent{Env: ecosystem.Environment{ID: g.dest}, Items: items})
		if err != nil {
			m.notice = "cannot resolve apply plan for " + displayPath(g.dest) + ": " + err.Error()
			return m, nil
		}
		for _, b := range plan.Batches {
			batches = append(batches, applyBatch{dest: g.dest, manager: g.manager, batch: b})
		}
	}
	if len(batches) == 0 {
		if m.planHasConflicts() {
			m.notice = "no operation can run — every one is blocked by an unresolved conflict; resolve them (r on a marked row) and re-apply"
			return m, nil
		}
		m.closeActiveTab() // nothing to apply: leave the plan tab
		return m, nil
	}
	var newly []string
	for _, d := range dests {
		wasHeld := false
		if m.locks != nil {
			wasHeld = m.locks.IsHeld(d)
			if err := m.locks.Acquire(d); err != nil {
				for _, n := range newly {
					m.locks.Release(n)
				}
				m.notice = heldNotice(d, err)
				return m, nil
			}
		}
		if !wasHeld {
			newly = append(newly, d)
		}
	}
	m.state.Applying = true
	m.applyBatches = batches
	m.applyBatchIdx = 0
	m.applyDests = dests
	m.applyLocks = newly
	m.applyFailed = 0
	m.applyFromTab = m.tabIdx
	m.applyDone = false
	m.applyAborted = false
	m.applyLog = nil
	for _, s := range conflictSkips {
		m.applyLog = append(m.applyLog, s)
	}
	for _, g := range m.planGroups() {
		if g.invalid {
			m.applyLog = append(m.applyLog, "skipped "+displayPath(g.dest)+" — destination is marked under two managers; clear one manager's marks there to run it")
		}
	}
	m.applyCurrent = batches[0].cmdLine()
	m.applyFrom = map[string]string{}
	for _, d := range dests {
		if ps := m.state.Prefixes[d]; ps != nil {
			for _, p := range ps.Packages {
				if p.HasMarks() {
					m.applyFrom[d+"\x00"+p.Name] = p.InstalledVersion
				}
			}
		}
	}
	// The apply run is an exclusive overlay: the tab strip is untouched for
	// its whole duration, so dismissing it returns to the opening tab.
	cmd := m.nextBatchCmd()
	return m, cmd
}

// nextBatchCmd runs the queued batch through its destination's own manager
// and captures its combined raw output so the apply screen can show the
// manager's log while the interface stays up. It stores the batch's cancel
// func so ctrl+c can abort a running invocation.
func (m *Model) nextBatchCmd() tea.Cmd {
	if m.applyBatchIdx >= len(m.applyBatches) {
		return m.finishApplyCmd()
	}
	ab := m.applyBatches[m.applyBatchIdx]
	idx := m.applyBatchIdx
	cmdLine := ab.cmdLine()
	dest := ab.dest
	manager := ab.manager
	batch := ab.batch
	eco := m.managers[manager]
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	m.applyCancel = cancel
	return func() tea.Msg {
		defer cancel()
		out, err := eco.Execute(ctx, ecosystem.Environment{ID: dest}, batch)
		return applyBatchMsg{idx: idx, cmdLine: cmdLine, output: out, err: err}
	}
}

// appendApplyLog records one finished batch (command header, raw output, and
// any error) in the log shown on the apply screen.
func (m *Model) appendApplyLog(cmdLine, output string, err error) {
	m.applyLog = append(m.applyLog, "$ "+cmdLine)
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	if len(lines) == 0 {
		m.applyLog = append(m.applyLog, "    (no output)")
	} else {
		for _, l := range lines {
			m.applyLog = append(m.applyLog, "    "+l)
		}
	}
	if err != nil {
		m.applyLog = append(m.applyLog, "    error: "+err.Error())
	}
}

// groupComplete reports whether ab is the last queued invocation of its
// (destination, manager) group; batches are queued group by group in order.
func (m Model) groupComplete(ab applyBatch) bool {
	return m.applyBatchIdx >= len(m.applyBatches) ||
		m.applyBatches[m.applyBatchIdx].dest != ab.dest ||
		m.applyBatches[m.applyBatchIdx].manager != ab.manager
}

// appendGroupResult records one finished group's outcome in the apply log so
// each group's result is reported even when other groups fail.
func (m *Model) appendGroupResult(ab applyBatch, ok bool, skipped int) {
	label := "group " + displayPath(ab.dest) + " via " + ab.manager
	if ok {
		m.applyLog = append(m.applyLog, label+": ok")
		return
	}
	if skipped > 0 {
		m.applyLog = append(m.applyLog, fmt.Sprintf("%s: FAILED — remaining %d invocation(s) not started", label, skipped))
		return
	}
	m.applyLog = append(m.applyLog, label+": FAILED")
}

// finishApplyCmd re-reads the actual on-disk state of every touched
// destination after the run instead of assuming the plan succeeded.
func (m Model) finishApplyCmd() tea.Cmd {
	dests := m.applyDests
	managerFor := map[string]ecosystem.Ecosystem{}
	for _, ab := range m.applyBatches {
		if _, ok := managerFor[ab.dest]; !ok {
			managerFor[ab.dest] = m.managers[ab.manager]
		}
	}
	return func() tea.Msg {
		pkgsByDest := map[string]map[string]*domain.PkgState{}
		errs := map[string]error{}
		for _, d := range dests {
			eco := managerFor[d]
			if eco == nil {
				eco = m.eco()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			list, err := eco.ListInstalled(ctx, ecosystem.Environment{ID: d})
			cancel()
			if err != nil {
				errs[d] = err
				continue
			}
			pkgsByDest[d] = toPkgStates(list)
		}
		return applyReloadMsg{pkgsByDest: pkgsByDest, errs: errs}
	}
}

// reconcileDestMarks clears marks whose effect is visible on disk after an
// apply run; operations that did not take effect keep their mark for a retry.
func (m *Model) reconcileDestMarks(dest string) {
	ps := m.state.Prefixes[dest]
	if ps == nil {
		return
	}
	for name, p := range ps.Packages {
		for manager, entry := range p.Marks {
			switch entry.Mark {
			case domain.MarkInstall:
				if p.Installed() {
					p.RevertFor(manager)
				}
			case domain.MarkRemove:
				if !p.Installed() {
					p.RevertFor(manager)
				}
			case domain.MarkUpgrade:
				from := m.applyFrom[dest+"\x00"+name]
				if (from != "" && p.InstalledVersion != from) ||
					(entry.TargetVersion != "" && p.InstalledVersion == entry.TargetVersion) {
					p.RevertFor(manager)
				}
			}
		}
	}
}

type sizesMsg struct {
	prefixID string
	name     string
	bytes    int64
}

type sizesDoneMsg struct {
	prefixID string
}

// measureSizesCmd walks each package directory in the background and emits a
// sizesMsg per package as it completes, so rows fill in progressively. dirOf
// maps a package name to its on-disk directory (layout is destination-kind
// dependent: global prefix vs project module dir).
func measureSizesCmd(prefixID string, names []string, dirOf func(name string) string) tea.Cmd {
	ch := make(chan tea.Msg, len(names)+1)
	go func() {
		defer close(ch)
		for _, name := range names {
			b, err := sizes.Measure(dirOf(name))
			if err != nil {
				continue
			}
			ch <- sizesMsg{prefixID: prefixID, name: name, bytes: b}
		}
	}()
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return sizesDoneMsg{prefixID: prefixID}
		}
		return msg
	}
}

// loadEnvCmd lists the environment's installed packages via the ecosystem.
func (m Model) loadEnvCmd(prefixID string) tea.Cmd {
	eco := m.eco()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		list, err := eco.ListInstalled(ctx, ecosystem.Environment{ID: prefixID})
		if err != nil {
			return loadEnvMsg{prefixID: prefixID, err: err}
		}
		return loadEnvMsg{prefixID: prefixID, pkgs: toPkgStates(list)}
	}
}

// applyLoaded merges freshly loaded packages into the prefix state,
// preserving pending marks and cached latest versions of packages that are
// still present. Rows absent from disk but carrying an install/upgrade mark
// (a pending operation) survive the reload so late loads cannot eat marks.
func (m *Model) applyLoaded(prefixID string, pkgs map[string]*domain.PkgState) {
	old := m.state.Prefixes[prefixID]
	if old != nil {
		for name, np := range pkgs {
			if op, ok := old.Packages[name]; ok {
				np.Marks = op.Marks
				np.LatestVersion = op.LatestVersion
			}
		}
		for name, op := range old.Packages {
			if _, ok := pkgs[name]; ok {
				continue
			}
			for _, e := range op.Marks {
				if e.Mark == domain.MarkInstall || e.Mark == domain.MarkUpgrade {
					pkgs[name] = op
					break
				}
			}
		}
	}
	ns := &domain.PrefixState{ID: prefixID, Packages: pkgs, Loaded: true}
	if old != nil {
		// Conflict state outlives a reload; the next resolve cycle refreshes it.
		ns.Conflicts = old.Conflicts
		ns.Chosen = old.Chosen
	}
	m.state.Prefixes[prefixID] = ns
	if prefixID == m.state.ActivePrefixID {
		m.closeSearchTabs() // the reload replaced the rows the search tabs referenced
	}
}

// displayRows is what the active list-like tab shows and its cursor moves
// over: one unified row per package name across all destinations of the
// active manager, or the registry-ordered results of a Search tab.
func (m Model) displayRows() []domain.UnifiedRow {
	return m.tabRows(m.activeTab())
}

// tabRows is the displayed rows of one list-like tab; the manual-only view
// drops automatic (A) rows after every other filter has applied.
func (m Model) tabRows(t Tab) []domain.UnifiedRow {
	var rows []domain.UnifiedRow
	if t.Kind == TabSearch {
		rows = m.searchDisplayRows(t)
	} else {
		rows = m.unifiedDisplayRows()
	}
	if m.manualOnly {
		kept := make([]domain.UnifiedRow, 0, len(rows))
		for _, u := range rows {
			if !u.Automatic() {
				kept = append(kept, u)
			}
		}
		rows = kept
	}
	return rows
}

// activePrefixes returns the loaded states and rank table of every
// destination of the active manager.
func (m Model) activePrefixes() ([]*domain.PrefixState, map[string]string) {
	var prefixes []*domain.PrefixState
	ranks := map[string]string{}
	for _, e := range m.envs() {
		if ps := m.state.Prefixes[e.ID]; ps != nil {
			prefixes = append(prefixes, ps)
			ranks[e.ID] = e.Rank
		}
	}
	return prefixes, ranks
}

// unifiedTotal counts the distinct package names across all destinations of
// the active manager (before any filter).
func (m Model) unifiedTotal() int {
	prefixes, ranks := m.activePrefixes()
	return len(domain.GroupByName(prefixes, ranks))
}

// unifiedDisplayRows aggregates every destination of the active manager by
// package name and applies the active filter, local match and sort.
func (m Model) unifiedDisplayRows() []domain.UnifiedRow {
	prefixes, ranks := m.activePrefixes()
	rows := domain.GroupByName(prefixes, ranks)
	if m.filterPred != nil {
		kept := make([]domain.UnifiedRow, 0, len(rows))
		for _, u := range rows {
			v := filter.View(u.Name, u.Installed(), u.Upgradable(), u.Unhealthy(), m.rowConflicted(u))
			if m.filterPred(v) {
				kept = append(kept, u)
			}
		}
		rows = kept
	}
	if m.prompt != nil && m.prompt.kind == PromptLocal {
		pat := strings.ToLower(m.prompt.input.Value())
		if pat != "" {
			kept := make([]domain.UnifiedRow, 0, len(rows))
			for _, u := range rows {
				if u.Installed() && strings.Contains(strings.ToLower(u.Name), pat) {
					kept = append(kept, u)
				}
			}
			rows = kept
		}
	}
	domain.SortUnified(rows, m.state.SortKey) // sorting applies to the local list only
	return rows
}

// searchDisplayRows keeps the registry result order of one Search tab; each
// result is a unified row without cross-destination aggregation. Marked
// leftovers from earlier queries and installed matches follow, name-sorted
// for stability.
func (m Model) searchDisplayRows(t Tab) []domain.UnifiedRow {
	ps := m.state.Active()
	if ps == nil {
		return nil
	}
	var rows []*domain.PkgState
	seen := make(map[string]bool, len(t.Order))
	for _, name := range t.Order {
		if p := ps.Packages[name]; p != nil && p.Origin == domain.OriginSearch {
			rows = append(rows, p)
			seen[name] = true
		}
	}
	var leftovers []*domain.PkgState
	var installed []*domain.PkgState
	for _, r := range ps.Rows() {
		if seen[r.Name] {
			continue
		}
		switch {
		case r.Origin == domain.OriginSearch:
			leftovers = append(leftovers, r)
		case r.Installed() && t.Hits[r.Name]:
			installed = append(installed, r)
		}
	}
	sort.Slice(leftovers, func(i, j int) bool { return leftovers[i].Name < leftovers[j].Name })
	sort.Slice(installed, func(i, j int) bool { return installed[i].Name < installed[j].Name })
	rows = append(append(rows, leftovers...), installed...)
	out := make([]domain.UnifiedRow, 0, len(rows))
	for _, p := range rows {
		u := domain.UnifiedRow{Name: p.Name, Row: p}
		if p.Installed() {
			u.Headline = p
			u.HeadlineID = ps.ID
		}
		out = append(out, u)
	}
	return out
}

// visibleRows projects the displayed unified rows onto their representative
// package rows.
func (m Model) visibleRows() []*domain.PkgState {
	rows := m.displayRows()
	out := make([]*domain.PkgState, 0, len(rows))
	for i := range rows {
		if rows[i].Row != nil {
			out = append(out, rows[i].Row)
		}
	}
	return out
}

// selectedUnified returns the unified row under the active tab's cursor, or
// nil.
func (m Model) selectedUnified() *domain.UnifiedRow {
	t := m.listTab()
	if t == nil {
		return nil
	}
	rows := m.displayRows()
	if t.Cursor >= len(rows) {
		return nil
	}
	return &rows[t.Cursor]
}

// markInstallOrUpgrade is aptitude's `+` semantics on the unified list: a
// not-installed package goes through install-target selection (spec: install
// target selection), an installed one is upgraded at its headline destination
// when that copy is outdated.
func (m *Model) markInstallOrUpgrade() {
	u := m.selectedUnified()
	if u == nil {
		return
	}
	if !u.Installed() {
		m.markInstallTargets(u.Name)
		return
	}
	h := u.Headline
	switch {
	case h.Upgradable():
		m.state.SetMark(u.HeadlineID, u.Name, m.activeManagerID, domain.MarkUpgrade)
	case u.Upgradable():
		m.notice = u.Name + " is upgradable on another destination — open the info screen to mark a specific one"
	case h.LatestVersion == "":
		m.notice = "latest version unknown for " + u.Name + " — press u to refresh"
	default:
		m.notice = u.Name + " is already at the latest version"
	}
}

// markRemove marks the selected row for removal at its headline destination
// (toggle); a package absent everywhere is ignored.
func (m *Model) markRemove() {
	u := m.selectedUnified()
	if u == nil || !u.Installed() {
		return
	}
	m.state.SetMark(u.HeadlineID, u.Name, m.activeManagerID, domain.MarkRemove)
}

// markHold marks every installed copy of the selected row as held, excluding
// them from bulk upgrades.
func (m *Model) markHold() {
	u := m.selectedUnified()
	if u == nil || !u.Installed() {
		return
	}
	for _, e := range m.envs() {
		ps := m.state.Prefixes[e.ID]
		if ps == nil {
			continue
		}
		if p := ps.Packages[u.Name]; p != nil && p.Installed() {
			p.SetMarkEntry(m.activeManagerID, domain.MarkEntry{Mark: domain.MarkHold})
		}
	}
}

// markRevert clears the active manager's pending mark on the selected row in
// every destination of the active manager (the row aggregates them).
func (m *Model) markRevert() {
	u := m.selectedUnified()
	if u == nil {
		return
	}
	for _, e := range m.envs() {
		m.state.Revert(e.ID, u.Name, m.activeManagerID)
	}
}

// markInstallTargets resolves the eligible destinations for installing name —
// those of the active manager where it is not installed and which are
// writable. Exactly one eligible destination records the mark directly (no
// popup); more than one opens the multi-select target popup.
func (m *Model) markInstallTargets(name string) {
	var eligible []ecosystem.Environment
	for _, e := range m.envs() {
		if !m.eco().Writable(e) {
			continue
		}
		if ps := m.state.Prefixes[e.ID]; ps != nil {
			if p := ps.Packages[name]; p != nil && p.Installed() {
				continue
			}
		}
		eligible = append(eligible, e)
	}
	if len(eligible) == 0 {
		m.notice = "no eligible destination to install " + name + " into"
		return
	}
	if len(eligible) == 1 {
		m.setInstallMarkForDest(eligible[0].ID, name)
		m.notice = name + " marked for install on " + displayPath(eligible[0].ID)
		return
	}
	sel := map[string]bool{}
	for _, e := range eligible {
		if ps := m.state.Prefixes[e.ID]; ps != nil {
			if p := ps.Packages[name]; p != nil && p.MarkFor(m.activeManagerID) == domain.MarkInstall {
				sel[e.ID] = true
			}
		}
	}
	m.installName = name
	m.installTargets = eligible
	m.installTargetSel = sel
	m.installTargetCursor = 0
	m.overlay = OverlayTargets
}

// setInstallMarkForDest records the active manager's install mark on name in
// one destination, creating the package row there when needed.
func (m *Model) setInstallMarkForDest(destID, name string) {
	ps := m.state.Prefixes[destID]
	if ps == nil {
		ps = &domain.PrefixState{ID: destID, Packages: map[string]*domain.PkgState{}}
		m.state.Prefixes[destID] = ps
	}
	p := ps.Packages[name]
	if p == nil {
		latest := ""
		if aps := m.state.Active(); aps != nil {
			if ap := aps.Packages[name]; ap != nil {
				latest = ap.LatestVersion
			}
		}
		p = &domain.PkgState{Name: name, Origin: domain.OriginSearch, LatestVersion: latest}
		ps.Packages[name] = p
	}
	p.SetMarkFor(m.activeManagerID, domain.MarkInstall)
}

// markDestInstall marks the info package for install on the destination under
// the cursor of the per-destination table.
func (m *Model) markDestInstall(t *Tab) {
	envs := m.envs()
	if t.DestCursor >= len(envs) {
		return
	}
	m.setInstallMarkForDest(envs[t.DestCursor].ID, t.Name)
}

// markDestRemove marks the info package for removal from the destination
// under the cursor of the per-destination table (toggle).
func (m *Model) markDestRemove(t *Tab) {
	envs := m.envs()
	if t.DestCursor >= len(envs) {
		return
	}
	e := envs[t.DestCursor]
	ps := m.state.Prefixes[e.ID]
	if ps == nil {
		m.notice = "not installed on " + displayPath(e.ID)
		return
	}
	p := ps.Packages[t.Name]
	if p == nil || !p.Installed() {
		m.notice = "not installed on " + displayPath(e.ID)
		return
	}
	m.state.SetMark(e.ID, t.Name, m.activeManagerID, domain.MarkRemove)
}

// submitFilter parses the prompt expression; on failure the previous filter
// stays active and an error notice is shown.
func (m *Model) submitFilter() {
	expr := strings.TrimSpace(m.prompt.input.Value())
	pred, err := filter.Parse(expr)
	if err != nil {
		m.notice = "invalid filter: " + err.Error()
		return
	}
	m.state.FilterText = expr
	m.filterPred = pred
	m.notice = ""
}
