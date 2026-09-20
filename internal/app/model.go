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

type Screen int

const (
	ScreenList Screen = iota
	ScreenPicker
	ScreenManager
	ScreenTargets
	ScreenPlan
	ScreenInfo
	ScreenVersions
	ScreenReadme
	ScreenHelp
)

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
	screen          Screen
	helpFrom        Screen // screen to return to when the help screen closes
	width           int
	height          int
	hostname        string
	cursor          int
	listTop         int // first visible row index of the list viewport
	notice          string
	filterPred      filter.Predicate
	prompt          *promptState
	pickerCursor    int
	pickerLocked    map[string]bool // env IDs held by another live instance
	activeFallback  string          // last resolved active environment (for vanished-prefix fallback)

	// searchActive is true while a registry search result set is on display;
	// the list then shows only those results (plus installed rows that match).
	// Results load in pages: reaching the end of the loaded rows fetches the
	// next page (infinite scroll), up to searchTotal.
	searchActive  bool
	searchQuery   string
	searchNames   map[string]bool
	searchOrder   []string // search rows of the current query, in registry order
	searchTotal   int      // total matches reported by the registry
	searchFetched int      // hits consumed for the current query (next page offset)
	searchLoading bool     // a page fetch is in flight

	locks *lock.Manager

	envsByManager map[string][]ecosystem.Environment // discovered destinations per manager
	managerCursor int                                // cursor on the manager switcher screen

	planSizes     map[string]int64 // name -> unpacked size shown in the plan screen
	applyBatches  []applyBatch     // queued (destination, manager) batches for the running apply
	applyBatchIdx int
	applyDests    []string          // every destination the running apply touches, in order
	applyLocks    []string          // destinations locked at apply start, released after it
	applyFrom     map[string]string // dest+\x00+name -> installed version at apply start
	applyFailed   int
	applyDone     bool               // completion prompt is showing
	applyLog      []string           // raw manager output lines shown on the apply screen
	applyCurrent  string             // command currently running (bottom line while applying)
	applyCancel   context.CancelFunc // aborts the batch currently running
	applyAborted  bool               // user pressed ctrl+c: the whole plan remainder is dropped

	installName         string                  // package the install-target popup marks
	installTargets      []ecosystem.Environment // eligible destinations in the popup
	installTargetCursor int
	installTargetSel    map[string]bool // destination IDs preselected/selected in the popup

	infoName       string // package the info screen shows
	infoDoc        *ecosystem.Doc
	infoLocal      bool   // doc came from the local package.json (offline)
	infoErr        string // set when metadata could not be fetched/read
	infoDestCursor int    // cursor on the info screen's per-destination table
	verCursor      int
	readmeLines    []string
	readmeScroll   int

	helpScroll int // scroll position on the help screen

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
		envsByManager:   map[string][]ecosystem.Environment{},
		locks:           lock.NewDefault(),
		hostname:        host,
	}
}

// NewProject builds a project-mode model (design D5): one entry per
// applicable manager, each bound to its own adapter instance; exactly the
// first applicable manager starts active. A project with no recognized
// markers still launches, showing a notice instead of guessing.
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
		envsByManager: map[string][]ecosystem.Environment{},
		locks:         lock.NewDefault(),
		hostname:      host,
	}
	if len(applicable) > 0 {
		m.activeManagerID = applicable[0]
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

// resetSearch clears the transient search-result view (marks are untouched;
// they live in their own PrefixState rows).
func (m *Model) resetSearch() {
	m.searchActive = false
	m.searchQuery = ""
	m.searchNames = nil
	m.searchOrder = nil
	m.searchTotal = 0
	m.searchFetched = 0
	m.searchLoading = false
}

// hasMoreSearch reports whether the active search still has pages to load.
func (m Model) hasMoreSearch() bool {
	return m.searchActive && !m.searchLoading && m.searchTotal > 0 && m.searchFetched < m.searchTotal
}

// loadMoreSearchCmd fetches the next page of the active search results.
func (m *Model) loadMoreSearchCmd() tea.Cmd {
	ps := m.state.Active()
	if ps == nil || !m.hasMoreSearch() {
		return nil
	}
	from := m.searchFetched
	m.searchLoading = true
	return m.searchCmd(ps.ID, m.searchQuery, from)
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
		out[p.Name] = &domain.PkgState{Name: p.Name, InstalledVersion: p.Version, Unhealthy: p.Unhealthy, Origin: domain.OriginInstalled}
	}
	return out
}

// applySearchResults merges one page of search hits into the active prefix's
// list and switches the view to search mode: installed rows are never
// duplicated or modified (they stay authoritative); previously displayed
// search rows without a pending mark are replaced by the first page, while
// later pages only add; marked search rows are retained regardless. While
// active, visibleRows shows only these results.
func (m *Model) applySearchResults(prefixID string, query string, from int, hits []ecosystem.Hit, total int) {
	ps := m.state.Prefixes[prefixID]
	if ps == nil {
		return
	}
	if from == 0 {
		for name, p := range ps.Packages {
			if p.Origin == domain.OriginSearch && !p.HasMarks() {
				delete(ps.Packages, name)
			}
		}
		m.searchFetched = 0
		m.searchNames = make(map[string]bool, len(hits))
		m.searchOrder = nil
	}
	ordered := make(map[string]bool, len(m.searchOrder))
	for _, n := range m.searchOrder {
		ordered[n] = true
	}
	for _, h := range hits {
		m.searchNames[h.Name] = true
		if p, ok := ps.Packages[h.Name]; ok {
			if p.Origin == domain.OriginSearch && !ordered[h.Name] {
				m.searchOrder = append(m.searchOrder, h.Name) // marked leftover at its registry rank
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
		m.searchOrder = append(m.searchOrder, h.Name)
		ordered[h.Name] = true
	}
	m.searchActive = true
	m.searchQuery = query
	m.searchFetched += len(hits)
	if total > 0 {
		m.searchTotal = total
	}
	m.searchLoading = false
}

// clearSearch leaves search mode and drops the unmarked search rows so the
// list returns to exactly the installed set (marked rows survive).
func (m *Model) clearSearch() {
	if ps := m.state.Active(); ps != nil {
		for name, p := range ps.Packages {
			if p.Origin == domain.OriginSearch && !p.HasMarks() {
				delete(ps.Packages, name)
			}
		}
	}
	m.resetSearch()
	m.clampCursor()
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
	m.cursor = 0
	m.notice = ""
	m.resetSearch()
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

// planGroups derives the ordered (destination, manager) groups of all pending
// marks across every live destination and manager. In project mode only the
// active adapter's marks participate (per-adapter isolation: a plan built
// under one adapter never carries another adapter's operations). A group
// whose destination carries marks of two different managers is flagged
// invalid (spec: one-manager-per-destination guard).
func (m Model) planGroups() []planGroup {
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
	plan := &domain.Plan{Ops: ops}
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

// openPlan shows the plan preview and starts fetching install sizes.
func (m Model) openPlan() (Model, tea.Cmd) {
	m.screen = ScreenPlan
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

// openInfo shows the info screen for the selected row and starts loading its
// metadata (local package.json when installed, registry document otherwise).
// The per-destination table cursor starts on the headline destination.
func (m Model) openInfo() (Model, tea.Cmd) {
	u := m.selectedUnified()
	if u == nil {
		return m, nil
	}
	m.screen = ScreenInfo
	m.infoName = u.Name
	m.infoDoc = nil
	m.infoLocal = false
	m.infoErr = ""
	envID := m.state.ActivePrefixID
	if u.HeadlineID != "" {
		envID = u.HeadlineID
	}
	m.infoDestCursor = 0
	for i, e := range m.envs() {
		if e.ID == envID {
			m.infoDestCursor = i
			break
		}
	}
	return m, m.infoCmd(envID, u.Name, u.Installed())
}

// openVersions shows the version history; when the current doc carries no
// version list (local doc of an installed package) it fetches the registry
// document first.
func (m Model) openVersions() (Model, tea.Cmd) {
	if m.infoDoc != nil && len(m.infoDoc.Versions) > 0 {
		m.screen = ScreenVersions
		m.positionVersionCursor()
		return m, nil
	}
	m.screen = ScreenVersions
	return m, m.versionsCmd(m.state.ActivePrefixID, m.infoName)
}

// positionVersionCursor puts the cursor on the installed version, else the
// latest, else the first entry.
func (m *Model) positionVersionCursor() {
	if m.infoDoc == nil || len(m.infoDoc.Versions) == 0 {
		m.verCursor = 0
		return
	}
	prefer := ""
	if ps := m.state.Active(); ps != nil {
		if p := ps.Packages[m.infoName]; p != nil && p.InstalledVersion != "" {
			prefer = p.InstalledVersion
		}
	}
	if prefer == "" {
		prefer = m.infoDoc.Latest
	}
	for i, v := range m.infoDoc.Versions {
		if v == prefer {
			m.verCursor = i
			return
		}
	}
	m.verCursor = 0
}

// pinVersion marks the package for install/upgrade at exactly the selected
// version (spec: version history).
func (m *Model) pinVersion() {
	if m.infoDoc == nil || m.verCursor >= len(m.infoDoc.Versions) {
		return
	}
	v := m.infoDoc.Versions[m.verCursor]
	ps := m.state.Active()
	if ps == nil {
		return
	}
	p := ps.Packages[m.infoName]
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
	m.screen = ScreenInfo
}

// openReadme shows the README view. Source priority (design D9): local README
// file for installed packages, the already-loaded registry readme, then an
// on-demand registry fetch; absent → a notice line.
func (m Model) openReadme() (Model, tea.Cmd) {
	var text string
	found := false
	if ps := m.state.Active(); ps != nil {
		if p := ps.Packages[m.infoName]; p != nil && p.Installed() {
			text, found = m.eco().Readme(ecosystem.Environment{ID: m.state.ActivePrefixID}, m.infoName)
		}
	}
	if !found && m.infoDoc != nil && m.infoDoc.Readme != "" {
		text, found = m.infoDoc.Readme, true
	}
	if found {
		m.setReadme(text)
		return m, nil
	}
	m.screen = ScreenReadme
	m.readmeLines = []string{"loading…"}
	return m, m.readmeFetchCmd(m.state.ActivePrefixID, m.infoName)
}

func (m *Model) setReadme(text string) {
	if text == "" {
		m.readmeLines = []string{"no README available"}
	} else {
		m.readmeLines = markdownToText(text)
	}
	m.readmeScroll = 0
	m.screen = ScreenReadme
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
// violations) are skipped and reported in the apply log.
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
	for _, g := range valid {
		eco := m.managers[g.manager]
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
		m.screen = ScreenList
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
	m.applyDone = false
	m.applyAborted = false
	m.applyLog = nil
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
	cmd := m.nextBatchCmd()
	m.screen = ScreenList
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
	m.state.Prefixes[prefixID] = &domain.PrefixState{ID: prefixID, Packages: pkgs, Loaded: true}
	if m.searchActive && prefixID == m.state.ActivePrefixID {
		m.resetSearch() // the reload replaced the rows the search view referenced
	}
}

// displayRows is what the list shows and the cursor moves over: one unified
// row per package name across all destinations of the active manager, or the
// registry-ordered search results while a search is active.
func (m Model) displayRows() []domain.UnifiedRow {
	if m.searchActive {
		return m.searchDisplayRows()
	}
	return m.unifiedDisplayRows()
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
			v := filter.View(u.Name, u.Installed(), u.Upgradable(), u.Unhealthy())
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

// searchDisplayRows keeps the registry result order of the active search;
// each result is a unified row without cross-destination aggregation. Marked
// leftovers from earlier queries and installed matches follow, name-sorted
// for stability.
func (m Model) searchDisplayRows() []domain.UnifiedRow {
	ps := m.state.Active()
	if ps == nil {
		return nil
	}
	var rows []*domain.PkgState
	seen := make(map[string]bool, len(m.searchOrder))
	for _, name := range m.searchOrder {
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
		case r.Installed() && m.searchNames[r.Name]:
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

// selectedUnified returns the unified row under the cursor, or nil.
func (m Model) selectedUnified() *domain.UnifiedRow {
	rows := m.displayRows()
	if m.cursor >= len(rows) {
		return nil
	}
	return &rows[m.cursor]
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
	m.screen = ScreenTargets
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
func (m *Model) markDestInstall() {
	envs := m.envs()
	if m.infoDestCursor >= len(envs) {
		return
	}
	m.setInstallMarkForDest(envs[m.infoDestCursor].ID, m.infoName)
}

// markDestRemove marks the info package for removal from the destination
// under the cursor of the per-destination table (toggle).
func (m *Model) markDestRemove() {
	envs := m.envs()
	if m.infoDestCursor >= len(envs) {
		return
	}
	e := envs[m.infoDestCursor]
	ps := m.state.Prefixes[e.ID]
	if ps == nil {
		m.notice = "not installed on " + displayPath(e.ID)
		return
	}
	p := ps.Packages[m.infoName]
	if p == nil || !p.Installed() {
		m.notice = "not installed on " + displayPath(e.ID)
		return
	}
	m.state.SetMark(e.ID, m.infoName, m.activeManagerID, domain.MarkRemove)
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
