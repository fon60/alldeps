package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"npmitude/internal/domain"
	"npmitude/internal/ecosystem"
	"npmitude/internal/sizes"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampCursor()
	case discoverMsg:
		m.envsByManager = msg.envsByManager
		for _, id := range m.managerIDs() {
			if err := msg.errs[id]; err != nil {
				m.notice = "manager " + id + " scan failed: " + err.Error()
			}
		}
		if m.state.ActivePrefixID == "" {
			envs := m.envs()
			id := ""
			for _, e := range envs {
				if e.Meta[ecosystem.MetaActive] == "1" {
					id = e.ID
					break
				}
			}
			if id == "" && len(envs) > 0 {
				id = envs[0].ID
			}
			if id == "" {
				return m, nil
			}
			m.activeFallback = id
			if m.locks != nil {
				if err := m.locks.Acquire(id); err != nil {
					m.notice = heldNotice(id, err)
					m.overlay = OverlayPicker
					m.pickerLocked = m.lockedEnvs()
					return m, nil
				}
			}
			m.state.ActivePrefixID = id
			if _, ok := m.state.Prefixes[id]; !ok {
				m.state.Prefixes[id] = &domain.PrefixState{ID: id, Packages: map[string]*domain.PkgState{}}
			}
			return m, m.loadAllEnvsCmd()
		}
		if m.overlay == OverlayPicker {
			m.pickerLocked = m.lockedEnvs()
			for i, e := range m.envs() {
				if !m.pickerLocked[e.ID] {
					m.pickerCursor = i
					break
				}
			}
		}
		return m, m.dropVanishedEnvs()
	case loadEnvMsg:
		if msg.err != nil {
			m.notice = "failed to load prefix " + msg.prefixID + ": " + msg.err.Error()
			return m, nil
		}
		m.applyLoaded(msg.prefixID, msg.pkgs)
		m.clampCursor()
		names := make([]string, 0, len(msg.pkgs))
		for name := range msg.pkgs {
			names = append(names, name)
		}
		dirOf := func(name string) string { return sizes.PackageDir(msg.prefixID, name) }
		if m.isProject() {
			// A project environment id is the module directory itself.
			dirOf = func(name string) string { return filepath.Join(msg.prefixID, name) }
		}
		cmds := []tea.Cmd{measureSizesCmd(msg.prefixID, names, dirOf)}
		if len(names) > 0 {
			cmds = append(cmds, m.checkOutdatedCmd(msg.prefixID, names))
		}
		return m, tea.Batch(cmds...)
	case outdatedMsg:
		ps := m.state.Prefixes[msg.prefixID]
		if ps == nil {
			return m, nil
		}
		for name, v := range msg.versions {
			if p := ps.Packages[name]; p != nil {
				p.LatestVersion = v
			}
		}
		if msg.err != nil {
			if !errors.Is(msg.err, ecosystem.ErrNoRegistry) {
				m.notice = "registry unreachable — upgradability unknown (list is local data only)"
			}
			return m, nil
		}
		if msg.total > 0 && msg.failed == msg.total {
			m.notice = "registry unreachable — upgradability unknown (list is local data only)"
		} else if msg.failed > 0 {
			m.notice = fmt.Sprintf("upgradability check: %d of %d packages failed", msg.failed, msg.total)
		}
	case searchMsg:
		idx := m.findTab(TabSearch, msg.query)
		if idx < 0 {
			return m, nil // stale: the search tab was closed or re-queried
		}
		t := &m.tabs[idx]
		if msg.err != nil {
			t.Loading = false
			if errors.Is(msg.err, ecosystem.ErrNoRegistry) {
				m.notice = "no registry configured for this prefix"
			} else if msg.from > 0 {
				m.notice = "failed to load more results: " + msg.err.Error()
			} else {
				m.notice = "search failed: " + msg.err.Error()
			}
			return m, nil
		}
		m.applySearchResults(msg.prefixID, msg.query, msg.from, msg.hits, msg.total)
		if msg.from == 0 && len(msg.hits) == 0 {
			m.notice = fmt.Sprintf("no results for %q", msg.query)
		} else if msg.from == 0 {
			m.notice = ""
		}
		m.clampTab(idx)
	case sizesMsg:
		if ps := m.state.Prefixes[msg.prefixID]; ps != nil {
			if p := ps.Packages[msg.name]; p != nil {
				b := msg.bytes
				p.SizeBytes = &b
			}
		}
	case sizesDoneMsg:
		if ps := m.state.Prefixes[msg.prefixID]; ps != nil {
			ps.SizesKnown = true
		}
	case planSizeMsg:
		if msg.err == nil && m.planSizes != nil {
			m.planSizes[msg.name] = msg.bytes
		}
	case resolveMsg:
		// A resolver failure is non-fatal: the previous conflict state stands.
		if msg.err != nil {
			return m, nil
		}
		ps := m.state.Prefixes[msg.dest]
		if ps == nil {
			return m, nil
		}
		fresh := make(map[string][]ecosystem.Conflict, len(msg.conflicts))
		for _, c := range msg.conflicts {
			fresh[c.Package] = append(fresh[c.Package], c)
			ps.ForgetResolution(c.Package) // the resolver still reports it: the pick did not settle it
		}
		ps.Conflicts = fresh
	case infoDataMsg:
		idx := m.findTab(TabInfo, msg.name)
		if idx < 0 {
			return m, nil // stale: the info tab was closed
		}
		t := &m.tabs[idx]
		if msg.err != nil {
			t.Err = "details unavailable: " + msg.err.Error()
			return m, nil
		}
		t.Doc = msg.doc
		t.Local = msg.local
	case versionsMsg:
		idx := m.findTab(TabVersions, msg.name)
		if idx < 0 {
			return m, nil // stale: the versions tab was closed
		}
		t := &m.tabs[idx]
		if msg.err != nil {
			if errors.Is(msg.err, ecosystem.ErrNoRegistry) {
				t.Err = "no registry configured for this prefix — version history unavailable"
			} else {
				t.Err = "version history unavailable: " + msg.err.Error()
			}
			return m, nil
		}
		t.Doc = msg.doc
		m.positionVersionCursor(idx)
	case readmeMsg:
		idx := m.findTab(TabReadme, msg.name)
		if idx < 0 {
			return m, nil // stale: the readme tab was closed
		}
		if msg.err != nil {
			if errors.Is(msg.err, ecosystem.ErrNoRegistry) {
				m.setReadmeLines(idx, "")
				return m, nil
			}
			m.tabs[idx].ReadmeLines = []string{fmt.Sprintf("readme unavailable: %s", msg.err)}
			return m, nil
		}
		m.setReadmeLines(idx, msg.text)
	case applyBatchMsg:
		ab := m.applyBatches[msg.idx]
		if msg.err != nil {
			m.applyFailed++
		}
		m.appendApplyLog(msg.cmdLine, msg.output, msg.err)
		m.applyCancel = nil
		m.applyBatchIdx = msg.idx + 1
		switch {
		case m.applyAborted:
			// User aborted: the entire remainder of the plan is dropped.
			m.applyBatchIdx = len(m.applyBatches)
			m.applyLog = append(m.applyLog, "aborted by user — remaining invocations not started")
		case msg.err != nil:
			// Per-group failure isolation: the rest of this group's
			// invocations are not started; other groups still run.
			skipped := 0
			for m.applyBatchIdx < len(m.applyBatches) &&
				m.applyBatches[m.applyBatchIdx].dest == ab.dest &&
				m.applyBatches[m.applyBatchIdx].manager == ab.manager {
				m.applyBatchIdx++
				skipped++
			}
			m.appendGroupResult(ab, false, skipped)
		case m.groupComplete(ab):
			m.appendGroupResult(ab, true, 0)
		}
		if m.applyBatchIdx < len(m.applyBatches) {
			m.applyCurrent = m.applyBatches[m.applyBatchIdx].cmdLine()
		} else {
			m.applyCurrent = ""
		}
		cmd := m.nextBatchCmd()
		return m, cmd
	case applyReloadMsg:
		m.state.Applying = false
		if len(msg.errs) > 0 {
			var names []string
			for d := range msg.errs {
				names = append(names, displayPath(d))
			}
			sort.Strings(names)
			m.notice = "apply finished but re-read failed for " + strings.Join(names, ", ")
		} else {
			for dest, pkgs := range msg.pkgsByDest {
				if old := m.state.Prefixes[dest]; old != nil {
					for name, p := range old.Packages {
						if hasInstallMark(p) && !p.Installed() {
							if _, ok := pkgs[name]; !ok {
								pkgs[name] = &domain.PkgState{
									Name:          p.Name,
									LatestVersion: p.LatestVersion,
									Description:   p.Description,
									Origin:        p.Origin,
									Marks:         p.Marks,
								}
							}
						}
					}
				}
				m.applyLoaded(dest, pkgs)
			}
			for _, dest := range m.applyDests {
				if _, ok := msg.pkgsByDest[dest]; ok {
					m.reconcileDestMarks(dest)
				}
			}
			if m.applyFailed > 0 {
				m.notice = fmt.Sprintf("apply finished with %d failed operation(s); list re-read from disk", m.applyFailed)
			} else {
				m.notice = "apply finished; list re-read from disk"
			}
		}
		for _, d := range m.applyLocks {
			if m.locks != nil {
				m.locks.Release(d)
			}
		}
		m.applyLocks = nil
		m.applyDone = true
		m.clampCursor()
		return m.withConflictRefresh()
	case tea.KeyMsg:
		if m.state.Applying || m.applyDone {
			// The apply screen is up: while the run is in progress only ctrl+c
			// acts (it aborts the running invocation); afterwards enter
			// continues and q quits.
			if !m.applyDone {
				if msg.String() == "ctrl+c" && m.applyCancel != nil {
					m.applyAborted = true
					m.applyCancel()
				}
				return m, nil
			}
			switch msg.String() {
			case "enter", " ":
				if m.applyFromTab < len(m.tabs) {
					m.tabIdx = m.applyFromTab // the run started from this tab; return to it
				}
				m.applyDone = false
				return m, nil
			case "q":
				m.releaseAll()
				return m, tea.Quit
			}
			return m, nil
		}
		if len(msg.Runes) > 1 {
			// A fast burst of plain keys can arrive coalesced into one KeyMsg;
			// process each rune as its own keypress in order.
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				single := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
				next, cmd := m.updateKey(single)
				m = next.(Model)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			return m.withConflictRefresh(cmds...)
		}
		next, cmd := m.updateKey(msg)
		return next.(Model).withConflictRefresh(cmd)
	}
	return m, nil
}

// updateKey routes a single keypress in fixed order (design D4): quit
// confirmation, then the focused prompt, then any open overlay, then the
// plan's conflict gate, then tab level (move / close), then the active
// tab's own keys.
func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.quitConfirm {
		switch msg.String() {
		case "y", "Y", "enter":
			m.releaseAll()
			return m, tea.Quit
		case "n", "N", "esc", "q":
			m.quitConfirm = false
		}
		return m, nil
	}
	if m.prompt != nil {
		return m.updatePrompt(msg)
	}
	switch m.overlay {
	case OverlayPicker:
		return m.updatePicker(msg)
	case OverlayManager:
		return m.updateManager(msg)
	case OverlayTargets:
		return m.updateTargets(msg)
	}
	if t := m.activeTab(); t.Kind == TabPlan && t.Gate {
		return m.updatePlanGate(msg)
	}
	switch msg.String() {
	case "ctrl+h", "ctrl+left":
		m.moveTab(-1)
		return m, nil
	case "ctrl+l", "ctrl+right":
		m.moveTab(1)
		return m, nil
	}
	if m.tabIdx > 0 && (msg.String() == "q" || msg.String() == "esc") {
		m.closeActiveTab()
		return m, nil
	}
	if msg.String() == "?" {
		m.openTab(TabHelp, "")
		return m, nil
	}
	switch m.activeTab().Kind {
	case TabPlan:
		return m.updatePlan(msg)
	case TabInfo:
		return m.updateInfo(msg)
	case TabVersions:
		return m.updateVersions(msg)
	case TabReadme:
		return m.updateReadme(msg)
	case TabResolver:
		return m.updateResolver(msg)
	case TabHelp:
		return m.updateHelp(msg)
	default:
		return m.updateList(msg)
	}
}

// updateList handles the keys shared by the List and Search tabs (both are
// list-like: a cursor over rows plus marks). q quits only here, because every
// other tab closes on q before its own keys run.
func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := &m.tabs[m.tabIdx]
	searching := t.Kind == TabSearch
	switch msg.String() {
	case "q", "Q":
		if m.state.TotalPending() > 0 {
			m.quitConfirm = true
			return m, nil
		}
		m.releaseAll()
		return m, tea.Quit
	case "E":
		m.overlay = OverlayPicker
		m.pickerLocked = m.lockedEnvs()
		m.pickerCursor = 0
		for i, e := range m.envs() {
			if e.ID == m.state.ActivePrefixID {
				m.pickerCursor = i
				break
			}
		}
	case "e":
		return m, m.cycleEnv()
	case "M":
		m.overlay = OverlayManager
		m.managerCursor = 0
		for i, id := range m.managerIDs() {
			if id == m.activeManagerID {
				m.managerCursor = i
				break
			}
		}
	case "g":
		if m.state.Applying {
			m.notice = "apply already in progress"
			return m, nil
		}
		if len(m.planGroups()) == 0 {
			m.notice = "no pending changes to apply"
			return m, nil
		}
		return m.openPlan()
	case "u":
		return m, m.refreshCmd()
	case "f":
		m.prompt = newPrompt(PromptFilter)
		m.prompt.input.Focus()
	case "l":
		if searching {
			m.notice = "the local match applies to the list tab — close the search tab first"
			return m, nil
		}
		m.prompt = newPrompt(PromptLocal)
		m.prompt.input.Focus()
	case "/":
		if !m.eco().Capabilities().HasSearch {
			m.notice = "search not available for this ecosystem"
			return m, nil
		}
		m.prompt = newPrompt(PromptSearch)
		m.prompt.input.Focus()
	case "+":
		m.markInstallOrUpgrade()
	case "-":
		m.markRemove()
	case "=":
		m.markHold()
	case ":":
		m.markRevert()
	case "r":
		if searching {
			m.notice = "conflict resolution applies to the list tab — close the search tab first"
			return m, nil
		}
		u := m.selectedUnified()
		if u == nil {
			return m, nil
		}
		dest := m.conflictDestFor(u.Name)
		if dest == "" {
			m.notice = u.Name + " has no unresolved conflict"
			return m, nil
		}
		return m.openResolver(dest, u.Name)
	case "U":
		n := 0
		for _, e := range m.envs() {
			n += m.state.MarkAllUpgradable(e.ID, m.activeManagerID)
		}
		if n == 0 {
			m.notice = "no upgradable packages to mark"
		} else {
			m.notice = fmt.Sprintf("%d packages marked for upgrade", n)
		}
	case "x":
		for _, e := range m.envs() {
			m.state.ClearAllMarks(e.ID, m.activeManagerID)
		}
		m.notice = "all marks cleared"
	case "v":
		u := m.selectedUnified()
		if u == nil {
			return m, nil
		}
		return m.openVersionsByName(u.Name)
	case "enter", "d":
		return m.openInfo()
	case "S":
		m.state.SortKey = m.state.SortKey.Next()
	case "up", "k":
		if t.Cursor > 0 {
			t.Cursor--
			m.syncListTop(m.tabIdx)
		}
	case "down", "j":
		if t.Cursor < len(m.displayRows())-1 {
			t.Cursor++
			m.syncListTop(m.tabIdx)
		} else if m.hasMoreSearch() {
			return m, m.loadMoreSearchCmd() // infinite scroll: load the next page
		}
	}
	return m, nil
}

// updatePicker handles keys while the environment picker overlay is open.
func (m Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.overlay = OverlayNone
		return m, nil
	case "up", "k":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
	case "down", "j":
		if m.pickerCursor < len(m.envs())-1 {
			m.pickerCursor++
		}
	case "enter":
		if m.pickerCursor < len(m.envs()) {
			id := m.envs()[m.pickerCursor].ID
			if m.pickerLocked[id] {
				m.notice = "environment " + displayPath(id) + " is open in another npmitude — choose a different environment"
				return m, nil
			}
			m.overlay = OverlayNone
			return m, m.switchEnv(id)
		}
	}
	return m, nil
}

// updateManager handles keys on the manager switcher. Selecting a manager
// changes only which mark-set is editable and which adapter executes;
// installed state (shared per destination) and all pending marks are kept.
// Destinations of the new manager that were never loaded start loading now —
// already-loaded ones are not re-fetched. In project mode each adapter owns
// its own destination, so switching also moves the environment lock from the
// old adapter's destinations to the new one's (acquired before release).
func (m Model) updateManager(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ids := m.managerIDs()
	switch msg.String() {
	case "esc", "q":
		m.overlay = OverlayNone
		return m, nil
	case "up", "k":
		if m.managerCursor > 0 {
			m.managerCursor--
		}
	case "down", "j":
		if m.managerCursor < len(ids)-1 {
			m.managerCursor++
		}
	case "enter":
		if m.managerCursor < len(ids) {
			id := ids[m.managerCursor]
			if id == m.activeManagerID {
				m.overlay = OverlayNone
				return m, nil
			}
			if m.isProject() && m.locks != nil {
				for _, e := range m.envsByManager[id] {
					if err := m.locks.Acquire(e.ID); err != nil {
						m.notice = heldNotice(e.ID, err)
						return m, nil
					}
				}
			}
			old := m.activeManagerID
			m.activeManagerID = id
			if m.isProject() && m.locks != nil {
				for _, e := range m.envsByManager[old] {
					m.locks.Release(e.ID)
				}
			}
			m.overlay = OverlayNone
			return m, m.loadAllEnvsCmd()
		}
	}
	return m, nil
}

// updateTargets handles keys on the install-target popup overlay:
// multi-select the eligible destinations (space toggles) and confirm with
// enter. Cancelling records nothing.
func (m Model) updateTargets(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.installTargets)
	switch msg.String() {
	case "esc", "q":
		m.overlay = OverlayNone
		return m, nil
	case "up", "k":
		if m.installTargetCursor > 0 {
			m.installTargetCursor--
		}
	case "down", "j":
		if m.installTargetCursor < n-1 {
			m.installTargetCursor++
		}
	case " ":
		if m.installTargetCursor < n {
			id := m.installTargets[m.installTargetCursor].ID
			m.installTargetSel[id] = !m.installTargetSel[id]
		}
	case "enter":
		changed := 0
		for _, e := range m.installTargets {
			marked := false
			if ps := m.state.Prefixes[e.ID]; ps != nil {
				if p := ps.Packages[m.installName]; p != nil && p.MarkFor(m.activeManagerID) == domain.MarkInstall {
					marked = true
				}
			}
			switch {
			case m.installTargetSel[e.ID] && !marked:
				m.setInstallMarkForDest(e.ID, m.installName)
				changed++
			case !m.installTargetSel[e.ID] && marked:
				m.state.Revert(e.ID, m.installName, m.activeManagerID)
				changed++
			}
		}
		if changed > 0 {
			m.notice = fmt.Sprintf("%s: install mark updated on %d destination(s)", m.installName, changed)
		} else {
			m.notice = "no changes"
		}
		m.overlay = OverlayNone
	}
	return m, nil
}

// updatePlanGate handles keys while the plan tab's conflict gate popup is up:
// [y] opens the resolver for the first affected cell, [n]/esc/q dismiss the
// gate and show the marked plan. The gate blocks every other key, including
// tab movement and closing the plan tab.
func (m Model) updatePlanGate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := &m.tabs[m.tabIdx]
	switch msg.String() {
	case "y", "Y", "enter":
		dest, name := m.nextConflictCell()
		if dest == "" {
			t.Gate = false
			return m, nil
		}
		return m.openResolver(dest, name)
	case "n", "N", "esc", "q":
		t.Gate = false
		return m, nil
	}
	return m, nil
}

// updatePlan handles keys on the plan tab (the gate is handled first): g or
// enter applies; n closes the tab (q/esc close it at the tab level). Cancelling
// leaves all marks pending and executes nothing.
func (m Model) updatePlan(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "g", "G", "enter":
		return m.startApply()
	case "n":
		m.closeActiveTab()
		return m, nil
	}
	return m, nil
}

// updateResolver handles keys on the resolver tab: j/k move over the flat
// option list of all conflicts of the cell, enter applies the option under
// the cursor (updating marks and re-resolving in the background). esc/q close
// the tab at the tab level.
func (m Model) updateResolver(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := &m.tabs[m.tabIdx]
	n := len(m.resolverOptionRows(t))
	switch msg.String() {
	case "up", "k":
		if t.RCursor > 0 {
			t.RCursor--
		}
	case "down", "j":
		if t.RCursor < n-1 {
			t.RCursor++
		}
	case "enter", " ":
		rows := m.resolverOptionRows(t)
		if t.RCursor >= len(rows) {
			return m, nil
		}
		m.applyResolutionOption(rows[t.RCursor])
	}
	return m, nil
}

// updateInfo handles keys on the info tab. j/k move the cursor over the
// per-destination table; + and - mark the package for install or removal
// against the destination under the cursor only; v/C open sibling tabs.
func (m Model) updateInfo(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := &m.tabs[m.tabIdx]
	n := len(m.envs())
	if t.DestCursor >= n && n > 0 {
		t.DestCursor = n - 1
	}
	switch msg.String() {
	case "enter", "d":
		m.closeActiveTab()
	case "v":
		return m.openVersions()
	case "C":
		return m.openReadme()
	case "up", "k":
		if t.DestCursor > 0 {
			t.DestCursor--
		}
	case "down", "j":
		if t.DestCursor < n-1 {
			t.DestCursor++
		}
	case "+":
		m.markDestInstall(t)
	case "-":
		m.markDestRemove(t)
	}
	return m, nil
}

// updateVersions handles keys on the versions tab: j/k move the cursor with a
// following viewport, g/G jump to first/last (readme-view parity), enter pins
// the version under the cursor.
func (m Model) updateVersions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := &m.tabs[m.tabIdx]
	n := 0
	if t.Doc != nil {
		n = len(t.Doc.Versions)
	}
	switch msg.String() {
	case "up", "k":
		if t.VerCursor > 0 {
			t.VerCursor--
			m.syncVersionTop(m.tabIdx)
		}
	case "down", "j":
		if t.VerCursor < n-1 {
			t.VerCursor++
			m.syncVersionTop(m.tabIdx)
		}
	case "g":
		t.VerCursor = 0
		m.syncVersionTop(m.tabIdx)
	case "G":
		if n > 0 {
			t.VerCursor = n - 1
			m.syncVersionTop(m.tabIdx)
		}
	case "enter", " ":
		m.pinVersion()
	}
	return m, nil
}

// updateReadme handles keys on the readme tab.
func (m Model) updateReadme(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := &m.tabs[m.tabIdx]
	h := m.readmeContentH()
	maxScroll := len(t.ReadmeLines) - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	switch msg.String() {
	case "enter":
		m.closeActiveTab()
	case "up", "k":
		if t.ReadmeScroll > 0 {
			t.ReadmeScroll--
		}
	case "down", "j":
		if t.ReadmeScroll < maxScroll {
			t.ReadmeScroll++
		}
	case "g":
		t.ReadmeScroll = 0
	case "G":
		t.ReadmeScroll = maxScroll
	}
	return m, nil
}

// updateHelp handles keys on the help tab; enter closes it (esc/q do at the
// tab level).
func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := &m.tabs[m.tabIdx]
	bodyH := m.height - 3
	if bodyH < 1 {
		bodyH = 1
	}
	maxScroll := len(helpContentLines()) - bodyH
	if maxScroll < 0 {
		maxScroll = 0
	}
	switch msg.String() {
	case "Q", "enter":
		m.closeActiveTab()
	case "up", "k":
		if t.HelpScroll > 0 {
			t.HelpScroll--
		}
	case "down", "j":
		if t.HelpScroll < maxScroll {
			t.HelpScroll++
		}
	case "g":
		t.HelpScroll = 0
	case "G":
		t.HelpScroll = maxScroll
	}
	return m, nil
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.prompt = nil
		m.clampCursor()
		return m, nil
	case "enter":
		var cmd tea.Cmd
		switch m.prompt.kind {
		case PromptFilter:
			m.submitFilter()
		case PromptSearch:
			query := strings.TrimSpace(m.prompt.input.Value())
			if query != "" {
				if ps := m.state.Active(); ps != nil {
					if at := m.activeTab(); at.Kind == TabSearch {
						// Re-query the active search tab in place: retarget its
						// subject, drop its loaded (unmarked) results, refetch 0.
						idx := m.tabIdx
						m.discardSearchRows(idx)
						t := &m.tabs[idx]
						t.Subject = query
						t.Query = query
						t.Hits = nil
						t.Order = nil
						t.Total = 0
						t.Fetched = 0
						t.Cursor = 0
						t.ListTop = 0
						t.Loading = true
						cmd = m.searchCmd(ps.ID, query, 0)
					} else {
						idx, created := m.openTab(TabSearch, query)
						if created {
							m.tabs[idx].Loading = true
							cmd = m.searchCmd(ps.ID, query, 0)
						}
					}
				} else {
					m.notice = "no registry configured for this prefix"
				}
			}
		}
		m.prompt = nil
		m.clampCursor()
		return m, cmd
	default:
		var cmd tea.Cmd
		m.prompt.input, cmd = m.prompt.input.Update(msg)
		if m.prompt.kind == PromptLocal {
			if lt := m.listTab(); lt != nil {
				lt.Cursor = 0
			}
		}
		return m, cmd
	}
}

// clampCursor clamps the active tab's cursor when it is list-like.
func (m *Model) clampCursor() {
	m.clampTab(m.tabIdx)
}

// clampTab keeps a list-like tab's cursor inside its displayed rows.
func (m *Model) clampTab(idx int) {
	t := &m.tabs[idx]
	if t.Kind != TabList && t.Kind != TabSearch {
		return
	}
	n := len(m.tabRows(*t))
	if t.Cursor > n-1 && n > 0 {
		t.Cursor = n - 1
	} else if n == 0 {
		t.Cursor = 0
	}
	m.syncListTop(idx)
}

// listHeight is the height of the list box; its first line is the column
// header, so it holds listHeight()-1 data rows.
func (m Model) listHeight() int {
	h := m.height - 3 - 5 // header(3) + description(3) + prompt(1) + status(1)
	if h < 1 {
		h = 1
	}
	return h
}

// syncListTop keeps a list-like tab's cursor inside the visible viewport: the
// list scrolls only when the cursor leaves the currently displayed range, in
// either direction. Visible data rows are one less than the box height because
// the column header occupies the first line of the box.
func (m *Model) syncListTop(idx int) {
	t := &m.tabs[idx]
	n := len(m.tabRows(*t))
	h := m.listHeight() - 1
	if h < 1 {
		h = 1
	}
	maxTop := n - h
	if maxTop < 0 {
		maxTop = 0
	}
	if t.Cursor < t.ListTop {
		t.ListTop = t.Cursor
	}
	if t.Cursor >= t.ListTop+h {
		t.ListTop = t.Cursor - h + 1
	}
	if t.ListTop > maxTop {
		t.ListTop = maxTop
	}
	if t.ListTop < 0 {
		t.ListTop = 0
	}
}
