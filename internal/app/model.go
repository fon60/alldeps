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
	state          *domain.AppState
	eco            ecosystem.Ecosystem
	screen         Screen
	helpFrom       Screen // screen to return to when the help screen closes
	width          int
	height         int
	hostname       string
	cursor         int
	listTop        int // first visible row index of the list viewport
	notice         string
	filterPred     filter.Predicate
	prompt         *promptState
	envs           []ecosystem.Environment
	pickerCursor   int
	pickerLocked   map[string]bool // env IDs held by another live instance
	activeFallback string          // last resolved active environment (for vanished-prefix fallback)

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

	planSizes    map[string]int64      // name -> unpacked size shown in the plan screen
	applyBatches []ecosystem.Batch     // queued batches for the running apply
	applyBatchIdx int
	applyFrom     map[string]string // name -> installed version at apply start
	applyFailed   int
	applyDone     bool               // completion prompt is showing
	applyLog      []string           // raw manager output lines shown on the apply screen
	applyCurrent  string             // command currently running (bottom line while applying)
	applyCancel   context.CancelFunc // aborts the batch currently running

	infoName     string // package the info screen shows
	infoDoc      *ecosystem.Doc
	infoLocal    bool   // doc came from the local package.json (offline)
	infoErr      string // set when metadata could not be fetched/read
	verCursor    int
	readmeLines  []string
	readmeScroll int

	helpScroll int // scroll position on the help screen

	quitConfirm bool // quit confirmation prompt is showing
}

// New builds the app model around the injected ecosystem adapter.
func New(eco ecosystem.Ecosystem) Model {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return Model{state: domain.NewAppState(), eco: eco, locks: lock.NewDefault(), hostname: host}
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
	for _, e := range m.envs {
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
	envs []ecosystem.Environment
	err  error
}

// discoverCmd asks the ecosystem for its destinations in the background.
func (m Model) discoverCmd() tea.Cmd {
	eco := m.eco
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		envs, err := eco.Discover(ctx)
		return discoverMsg{envs: envs, err: err}
	}
}

// refreshCmd re-discovers environments and reloads the active one from disk.
func (m Model) refreshCmd() tea.Cmd {
	active := m.state.ActivePrefixID
	eco := m.eco
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		envs, scanErr := eco.Discover(ctx)
		var loadErr error
		var pkgs map[string]*domain.PkgState
		if active != "" {
			list, err := eco.ListInstalled(ctx, ecosystem.Environment{ID: active})
			if err != nil {
				loadErr = err
			} else {
				pkgs = toPkgStates(list)
			}
		}
		return refreshMsg{envs: envs, scanErr: scanErr, prefixID: active, pkgs: pkgs, loadErr: loadErr}
	}
}

type refreshMsg struct {
	envs     []ecosystem.Environment
	scanErr  error
	prefixID string
	pkgs     map[string]*domain.PkgState
	loadErr  error
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
	eco := m.eco
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
			if p.Origin == domain.OriginSearch && p.Mark == domain.MarkNone {
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
			if p.Origin == domain.OriginSearch && p.Mark == domain.MarkNone {
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
	eco := m.eco
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		versions, failed, err := eco.LatestVersions(ctx, ecosystem.Environment{ID: prefixID}, names)
		return outdatedMsg{prefixID: prefixID, versions: versions, failed: failed, total: len(names), err: err}
	}
}

func (m Model) Init() tea.Cmd {
	return m.discoverCmd()
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
	if len(m.envs) == 0 {
		return nil
	}
	live := map[string]bool{}
	for _, e := range m.envs {
		live[e.ID] = true
	}
	active := m.state.ActivePrefixID
	if active != "" && !live[active] {
		fallback := m.activeFallback
		if fallback == "" || !live[fallback] {
			fallback = m.envs[0].ID
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
	if len(m.envs) == 0 {
		return nil
	}
	idx := -1
	for i, e := range m.envs {
		if e.ID == m.state.ActivePrefixID {
			idx = i
			break
		}
	}
	next := m.envs[(idx+1)%len(m.envs)]
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
	eco := m.eco
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
	eco := m.eco
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
	eco := m.eco
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
	eco := m.eco
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
	prefixID string
	pkgs     map[string]*domain.PkgState
	err      error
}

// targetVersion is the version an install/upgrade mark will apply: a pinned
// version if set, else the known latest.
func targetVersion(p *domain.PkgState) string {
	if p.TargetVersion != "" {
		return p.TargetVersion
	}
	return p.LatestVersion
}

// planRows splits the active prefix's marked packages by operation kind.
func (m Model) planRows() (installs, removals, upgrades []*domain.PkgState) {
	ps := m.state.Active()
	if ps == nil {
		return
	}
	for _, p := range ps.Rows() {
		switch p.Mark {
		case domain.MarkInstall:
			installs = append(installs, p)
		case domain.MarkRemove:
			removals = append(removals, p)
		case domain.MarkUpgrade:
			upgrades = append(upgrades, p)
		}
	}
	return
}

type planGroup struct {
	title string
	rows  []*domain.PkgState
}

// buildPlan groups pending marks for the plan preview screen.
func (m Model) buildPlan() []planGroup {
	installs, removals, upgrades := m.planRows()
	var groups []planGroup
	if len(installs) > 0 {
		groups = append(groups, planGroup{"Install", installs})
	}
	if len(removals) > 0 {
		groups = append(groups, planGroup{"Remove", removals})
	}
	if len(upgrades) > 0 {
		groups = append(groups, planGroup{"Upgrade", upgrades})
	}
	return groups
}

// openPlan shows the plan preview and starts fetching install sizes.
func (m Model) openPlan() (Model, tea.Cmd) {
	m.screen = ScreenPlan
	m.planSizes = map[string]int64{}
	var cmds []tea.Cmd
	if ps := m.state.Active(); ps != nil {
		for _, p := range ps.Packages {
			if p.Mark == domain.MarkInstall {
				cmds = append(cmds, m.planSizeCmd(ps.ID, p.Name, targetVersion(p)))
			}
		}
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

// openInfo shows the info screen for the selected row and starts loading its
// metadata (local package.json when installed, registry document otherwise).
func (m Model) openInfo() (Model, tea.Cmd) {
	r := m.selectedRow()
	if r == nil {
		return m, nil
	}
	m.screen = ScreenInfo
	m.infoName = r.Name
	m.infoDoc = nil
	m.infoLocal = false
	m.infoErr = ""
	return m, m.infoCmd(m.state.ActivePrefixID, r.Name, r.Installed())
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
		p.Mark = domain.MarkUpgrade
		m.notice = fmt.Sprintf("%s marked for upgrade to %s", p.Name, v)
	} else {
		p.Mark = domain.MarkInstall
		m.notice = fmt.Sprintf("%s marked for install at %s", p.Name, v)
	}
	p.TargetVersion = v
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
			text, found = m.eco.Readme(ecosystem.Environment{ID: m.state.ActivePrefixID}, m.infoName)
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

// buildIntent collects the active environment's pending marks into a
// manager-agnostic intent for the ecosystem resolver.
func (m Model) buildIntent() ecosystem.Intent {
	installs, removals, upgrades := m.planRows()
	items := make([]ecosystem.MarkedItem, 0, len(installs)+len(upgrades)+len(removals))
	for _, p := range installs {
		items = append(items, ecosystem.MarkedItem{Op: ecosystem.OpInstall, Name: p.Name, Version: targetVersion(p)})
	}
	for _, p := range upgrades {
		items = append(items, ecosystem.MarkedItem{Op: ecosystem.OpUpgrade, Name: p.Name, Version: targetVersion(p)})
	}
	for _, p := range removals {
		items = append(items, ecosystem.MarkedItem{Op: ecosystem.OpRemove, Name: p.Name})
	}
	return ecosystem.Intent{Env: ecosystem.Environment{ID: m.state.ActivePrefixID}, Items: items}
}

// startApply begins the single-flight apply run: it snapshots versions for
// post-apply reconciliation and launches the first batch.
func (m Model) startApply() (Model, tea.Cmd) {
	if m.state.Applying {
		m.notice = "apply already in progress"
		return m, nil
	}
	envID := m.state.ActivePrefixID
	if !m.eco.Writable(ecosystem.Environment{ID: envID}) {
		m.notice = fmt.Sprintf("prefix %s is not writable — make it writable or switch environments before applying", displayPath(envID))
		return m, nil
	}
	plan, _, err := m.eco.Resolve(m.buildIntent())
	if err != nil {
		m.notice = "cannot resolve apply plan: " + err.Error()
		return m, nil
	}
	batches := plan.Batches
	if len(batches) == 0 {
		m.screen = ScreenList
		return m, nil
	}
	m.state.Applying = true
	m.applyBatches = batches
	m.applyBatchIdx = 0
	m.applyFailed = 0
	m.applyDone = false
	m.applyLog = nil
	m.applyCurrent = batches[0].Label
	m.applyFrom = map[string]string{}
	if ps := m.state.Active(); ps != nil {
		for _, p := range ps.Packages {
			if p.Mark != domain.MarkNone {
				m.applyFrom[p.Name] = p.InstalledVersion
			}
		}
	}
	cmd := m.nextBatchCmd()
	m.screen = ScreenList
	return m, cmd
}

// nextBatchCmd runs the queued batch through the ecosystem and captures its
// combined raw output so the apply screen can show the manager's log while
// the interface stays up. It stores the batch's cancel func so ctrl+c can
// abort a running invocation.
func (m *Model) nextBatchCmd() tea.Cmd {
	if m.applyBatchIdx >= len(m.applyBatches) {
		return m.finishApplyCmd()
	}
	batch := m.applyBatches[m.applyBatchIdx]
	idx := m.applyBatchIdx
	cmdLine := batch.Label
	envID := m.state.ActivePrefixID
	eco := m.eco
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	m.applyCancel = cancel
	return func() tea.Msg {
		defer cancel()
		out, err := eco.Execute(ctx, ecosystem.Environment{ID: envID}, batch)
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

// finishApplyCmd re-reads the actual on-disk state after the run instead of
// assuming the plan succeeded.
func (m Model) finishApplyCmd() tea.Cmd {
	id := m.state.ActivePrefixID
	eco := m.eco
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		list, err := eco.ListInstalled(ctx, ecosystem.Environment{ID: id})
		if err != nil {
			return applyReloadMsg{prefixID: id, err: err}
		}
		return applyReloadMsg{prefixID: id, pkgs: toPkgStates(list)}
	}
}

// reconcileMarks clears marks whose effect is visible on disk after an apply
// run; operations that did not take effect keep their mark for a retry.
func (m *Model) reconcileMarks() {
	ps := m.state.Active()
	if ps == nil {
		return
	}
	for name, p := range ps.Packages {
		if p.Mark == domain.MarkNone {
			continue
		}
		switch p.Mark {
		case domain.MarkInstall:
			if p.Installed() {
				m.state.Revert(ps.ID, name)
			}
		case domain.MarkRemove:
			if !p.Installed() {
				m.state.Revert(ps.ID, name)
			}
		case domain.MarkUpgrade:
			from := m.applyFrom[name]
			if (from != "" && p.InstalledVersion != from) ||
				(p.TargetVersion != "" && p.InstalledVersion == p.TargetVersion) {
				m.state.Revert(ps.ID, name)
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
// sizesMsg per package as it completes, so rows fill in progressively.
func measureSizesCmd(prefixID string, names []string) tea.Cmd {
	ch := make(chan tea.Msg, len(names)+1)
	go func() {
		defer close(ch)
		for _, name := range names {
			b, err := sizes.Measure(sizes.PackageDir(prefixID, name))
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
	eco := m.eco
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
// still present.
func (m *Model) applyLoaded(prefixID string, pkgs map[string]*domain.PkgState) {
	old := m.state.Prefixes[prefixID]
	if old != nil {
		for name, np := range pkgs {
			if op, ok := old.Packages[name]; ok {
				np.Mark = op.Mark
				np.TargetVersion = op.TargetVersion
				np.LatestVersion = op.LatestVersion
			}
		}
	}
	m.state.Prefixes[prefixID] = &domain.PrefixState{ID: prefixID, Packages: pkgs, Loaded: true}
	if m.searchActive && prefixID == m.state.ActivePrefixID {
		m.resetSearch() // the reload replaced the rows the search view referenced
	}
}

// visibleRows returns the rows to render for the active prefix, with the
// active filter and any transient local match applied.
func (m Model) visibleRows() []*domain.PkgState {
	ps := m.state.Active()
	if ps == nil {
		return nil
	}
	rows := ps.Rows()
	if m.searchActive {
		// Registry results keep the order returned by the registry (pages are
		// appended); they are never re-sorted. Marked leftovers from earlier
		// queries and installed matches follow, name-sorted for stability.
		kept := make([]*domain.PkgState, 0, len(rows))
		seen := make(map[string]bool, len(m.searchOrder))
		for _, name := range m.searchOrder {
			if p := ps.Packages[name]; p != nil && p.Origin == domain.OriginSearch {
				kept = append(kept, p)
				seen[name] = true
			}
		}
		var leftovers []*domain.PkgState
		var installed []*domain.PkgState
		for _, r := range rows {
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
		rows = append(append(kept, leftovers...), installed...)
	}
	if m.filterPred != nil {
		kept := make([]*domain.PkgState, 0, len(rows))
		for _, r := range rows {
			v := filter.View(r.Name, r.Installed(), r.Upgradable(), r.Unhealthy)
			if m.filterPred(v) {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	if m.prompt != nil && m.prompt.kind == PromptLocal {
		pat := strings.ToLower(m.prompt.input.Value())
		if pat != "" {
			kept := make([]*domain.PkgState, 0, len(rows))
			for _, r := range rows {
				if r.Installed() && strings.Contains(strings.ToLower(r.Name), pat) {
					kept = append(kept, r)
				}
			}
			rows = kept
		}
	}
	if !m.searchActive {
		domain.SortRows(rows, m.state.SortKey) // sorting applies to the local list only
	}
	return rows
}

// selectedRow returns the package under the cursor, or nil.
func (m *Model) selectedRow() *domain.PkgState {
	rows := m.visibleRows()
	if m.cursor >= len(rows) {
		return nil
	}
	return rows[m.cursor]
}

// markInstallOrUpgrade is aptitude's `+` semantics: install a not-installed
// package, or upgrade an installed one that has a newer version known.
func (m *Model) markInstallOrUpgrade() {
	r := m.selectedRow()
	if r == nil {
		return
	}
	switch {
	case !r.Installed():
		m.state.SetMark(m.state.ActivePrefixID, r.Name, domain.MarkInstall)
	case r.Upgradable():
		m.state.SetMark(m.state.ActivePrefixID, r.Name, domain.MarkUpgrade)
	case r.LatestVersion == "":
		m.notice = "latest version unknown for " + r.Name + " — press u to refresh"
	default:
		m.notice = r.Name + " is already at the latest version"
	}
}

// markRemove marks the selected installed row for removal (toggle).
func (m *Model) markRemove() {
	r := m.selectedRow()
	if r == nil || !r.Installed() {
		return
	}
	m.state.SetMark(m.state.ActivePrefixID, r.Name, domain.MarkRemove)
}

// markHold marks the selected row as held, excluding it from bulk upgrades.
func (m *Model) markHold() {
	r := m.selectedRow()
	if r == nil {
		return
	}
	m.state.SetMark(m.state.ActivePrefixID, r.Name, domain.MarkHold)
}

// markRevert clears any pending mark on the selected row.
func (m *Model) markRevert() {
	r := m.selectedRow()
	if r == nil {
		return
	}
	m.state.Revert(m.state.ActivePrefixID, r.Name)
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
