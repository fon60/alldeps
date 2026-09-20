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
					m.screen = ScreenPicker
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
		if m.screen == ScreenPicker {
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
		if msg.err != nil {
			m.searchLoading = false
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
		m.clampCursor()
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
	case infoDataMsg:
		if msg.name != m.infoName || m.screen != ScreenInfo && m.screen != ScreenVersions {
			return m, nil // stale: the user moved on
		}
		if msg.err != nil {
			m.infoErr = "details unavailable: " + msg.err.Error()
			return m, nil
		}
		m.infoDoc = msg.doc
		m.infoLocal = msg.local
	case versionsMsg:
		if msg.name != m.infoName || m.screen != ScreenVersions {
			return m, nil // stale: the user moved on
		}
		if msg.err != nil {
			if errors.Is(msg.err, ecosystem.ErrNoRegistry) {
				m.infoErr = "no registry configured for this prefix — version history unavailable"
			} else {
				m.infoErr = "version history unavailable: " + msg.err.Error()
			}
			m.screen = ScreenInfo
			return m, nil
		}
		m.infoDoc = msg.doc
		m.positionVersionCursor()
	case readmeMsg:
		if msg.name != m.infoName || m.screen != ScreenReadme {
			return m, nil // stale: the user moved on
		}
		if msg.err != nil {
			if errors.Is(msg.err, ecosystem.ErrNoRegistry) {
				m.setReadme("")
				return m, nil
			}
			m.readmeLines = []string{fmt.Sprintf("readme unavailable: %s", msg.err)}
			return m, nil
		}
		m.setReadme(msg.text)
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
			return m, tea.Batch(cmds...)
		}
		return m.updateKey(msg)
	}
	return m, nil
}

// updateKey routes a single keypress.
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
	if msg.String() == "?" {
		m.helpFrom = m.screen
		m.screen = ScreenHelp
		m.helpScroll = 0
		return m, nil
	}
	if m.screen == ScreenPicker {
		return m.updatePicker(msg)
	}
	if m.screen == ScreenManager {
		return m.updateManager(msg)
	}
	if m.screen == ScreenTargets {
		return m.updateTargets(msg)
	}
	if m.screen == ScreenPlan {
		return m.updatePlan(msg)
	}
	if m.screen == ScreenInfo {
		return m.updateInfo(msg)
	}
	if m.screen == ScreenVersions {
		return m.updateVersions(msg)
	}
	if m.screen == ScreenReadme {
		return m.updateReadme(msg)
	}
	if m.screen == ScreenHelp {
		return m.updateHelp(msg)
	}
	switch msg.String() {
	case "esc":
		if m.searchActive {
			m.clearSearch()
			m.notice = "search cleared — showing installed packages"
		}
	case "q", "Q":
		if m.state.TotalPending() > 0 {
			m.quitConfirm = true
			return m, nil
		}
		m.releaseAll()
		return m, tea.Quit
	case "E":
		m.screen = ScreenPicker
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
		m.screen = ScreenManager
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
		if m.searchActive {
			m.notice = "clear the search first (esc), then match installed packages"
			return m, nil
		}
		m.prompt = newPrompt(PromptLocal)
		m.prompt.input.Focus()
	case "/":
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
	case "enter", "d":
		return m.openInfo()
	case "S":
		m.state.SortKey = m.state.SortKey.Next()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.syncListTop()
		}
	case "down", "j":
		if m.cursor < len(m.visibleRows())-1 {
			m.cursor++
			m.syncListTop()
		} else if m.hasMoreSearch() {
			return m, m.loadMoreSearchCmd() // infinite scroll: load the next page
		}
	}
	return m, nil
}

// updatePicker handles keys while the environment picker is open.
func (m Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.screen = ScreenList
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
			m.screen = ScreenList
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
		m.screen = ScreenList
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
				m.screen = ScreenList
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
			m.screen = ScreenList
			return m, m.loadAllEnvsCmd()
		}
	}
	return m, nil
}

// updateTargets handles keys on the install-target popup: multi-select the
// eligible destinations (space toggles) and confirm with enter. Cancelling
// records nothing.
func (m Model) updateTargets(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.installTargets)
	switch msg.String() {
	case "esc", "q":
		m.screen = ScreenList
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
		m.screen = ScreenList
	}
	return m, nil
}

// updatePlan handles keys while the plan preview is open. Cancelling leaves
// all marks pending and executes nothing. Confirming with g makes "g,g" a
// quick shortcut: open the plan, apply it.
func (m Model) updatePlan(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "g", "G", "enter":
		return m.startApply()
	case "n", "esc", "q":
		m.screen = ScreenList
		return m, nil
	}
	return m, nil
}

// updateInfo handles keys on the info screen. j/k move the cursor over the
// per-destination table; + and - mark the package for install or removal
// against the destination under the cursor only.
func (m Model) updateInfo(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.envs())
	if m.infoDestCursor >= n && n > 0 {
		m.infoDestCursor = n - 1
	}
	switch msg.String() {
	case "esc", "q", "enter", "d":
		m.screen = ScreenList
	case "v":
		return m.openVersions()
	case "C":
		return m.openReadme()
	case "up", "k":
		if m.infoDestCursor > 0 {
			m.infoDestCursor--
		}
	case "down", "j":
		if m.infoDestCursor < n-1 {
			m.infoDestCursor++
		}
	case "+":
		m.markDestInstall()
	case "-":
		m.markDestRemove()
	}
	return m, nil
}

// updateVersions handles keys on the version history screen.
func (m Model) updateVersions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := 0
	if m.infoDoc != nil {
		n = len(m.infoDoc.Versions)
	}
	switch msg.String() {
	case "esc", "q":
		m.screen = ScreenInfo
	case "up", "k":
		if m.verCursor > 0 {
			m.verCursor--
		}
	case "down", "j":
		if m.verCursor < n-1 {
			m.verCursor++
		}
	case "enter", " ":
		m.pinVersion()
	}
	return m, nil
}

// updateReadme handles keys on the README screen.
func (m Model) updateReadme(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	h := m.readmeContentH()
	maxScroll := len(m.readmeLines) - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	switch msg.String() {
	case "esc", "q", "enter":
		m.screen = ScreenInfo
	case "up", "k":
		if m.readmeScroll > 0 {
			m.readmeScroll--
		}
	case "down", "j":
		if m.readmeScroll < maxScroll {
			m.readmeScroll++
		}
	case "g":
		m.readmeScroll = 0
	case "G":
		m.readmeScroll = maxScroll
	}
	return m, nil
}

// updateHelp handles keys on the help screen; any of the close keys returns
// to the screen it was opened from.
func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	maxScroll := len(helpContentLines()) - bodyH
	if maxScroll < 0 {
		maxScroll = 0
	}
	switch msg.String() {
	case "esc", "q", "Q", "enter":
		m.screen = m.helpFrom
	case "up", "k":
		if m.helpScroll > 0 {
			m.helpScroll--
		}
	case "down", "j":
		if m.helpScroll < maxScroll {
			m.helpScroll++
		}
	case "g":
		m.helpScroll = 0
	case "G":
		m.helpScroll = maxScroll
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
					cmd = m.searchCmd(ps.ID, query, 0)
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
			m.cursor = 0
		}
		return m, cmd
	}
}

func (m *Model) clampCursor() {
	if n := len(m.visibleRows()); m.cursor > n-1 && n > 0 {
		m.cursor = n - 1
	} else if n == 0 {
		m.cursor = 0
	}
	m.syncListTop()
}

// listHeight is the number of package rows that fit in the list viewport.
func (m Model) listHeight() int {
	h := m.height - 2 - 5 // header(2) + description(3) + prompt(1) + status(1)
	if h < 1 {
		h = 1
	}
	return h
}

// syncListTop keeps the cursor inside the visible viewport: the list scrolls
// only when the cursor leaves the currently displayed range, in either
// direction.
func (m *Model) syncListTop() {
	n := len(m.visibleRows())
	h := m.listHeight()
	maxTop := n - h
	if maxTop < 0 {
		maxTop = 0
	}
	if m.cursor < m.listTop {
		m.listTop = m.cursor
	}
	if m.cursor >= m.listTop+h {
		m.listTop = m.cursor - h + 1
	}
	if m.listTop > maxTop {
		m.listTop = maxTop
	}
	if m.listTop < 0 {
		m.listTop = 0
	}
}
