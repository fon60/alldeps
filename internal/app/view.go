package app

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"npmitude/internal/domain"
	"npmitude/internal/ecosystem"
)

const (
	colFlag  = 4
	colSize  = 8
	colVer   = 12
	colCand  = 12
	colCount = 5 // presence counter: how many further destinations carry the package
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "npmitude — waiting for terminal size…"
	}
	var body string
	switch m.screen {
	case ScreenPicker:
		body = m.pickerBody()
	case ScreenManager:
		body = m.managerBody()
	case ScreenTargets:
		body = m.targetsBody()
	case ScreenPlan:
		body = m.planBody()
	case ScreenInfo:
		body = m.infoBody()
	case ScreenVersions:
		body = m.versionsBody()
	case ScreenReadme:
		body = m.readmeBody()
	case ScreenResolver:
		body = m.resolverBody()
	case ScreenHelp:
		body = m.helpBody()
	default:
		if m.state.Applying || m.applyDone {
			body = m.applyBody()
		} else {
			body = lipgloss.JoinVertical(lipgloss.Left,
				m.listRegion(m.listHeight()),
				m.descRegion(),
				m.promptLine(),
				m.statusLine(),
			)
		}
	}
	lines := append(m.headerLines(), body)
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(
		lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// headerLines is the fixed top of every screen: line 1 carries the app name,
// version and host; line 2 lists the actions available on the current screen.
// Both lines sit on the primary-color background band with white text.
func (m Model) headerLines() []string {
	title := fmt.Sprintf("npmitude %s @ %s", Version, m.hostname)
	return []string{
		headerTitleStyle.Width(m.width).Render(fitText(title, m.width)),
		headerHintStyle.Width(m.width).Render(fitText(m.screenHints(), m.width)),
	}
}

func (m Model) screenHints() string {
	if m.state.Applying || m.applyDone {
		if m.applyDone {
			return "[enter] continue   [q] quit"
		}
		return "applying changes — please wait"
	}
	switch m.screen {
	case ScreenPicker:
		return "enter: switch environment   esc/q: back"
	case ScreenManager:
		return "enter: select manager   esc/q: back"
	case ScreenTargets:
		return "space: toggle   enter: confirm   esc/q: cancel"
	case ScreenPlan:
		if m.planGate {
			return "[y] resolve conflicts   [n] show the marked plan"
		}
		return "[g] apply   [n/esc] cancel"
	case ScreenResolver:
		return "[j/k] choose option   [enter] apply it   [esc/q] back"
	case ScreenInfo:
		return "[esc] back   [v] versions   [C] readme"
	case ScreenVersions:
		return "[enter] pin version   [j/k] move   [esc] back"
	case ScreenReadme:
		return "[j/k] scroll   [g/G] top/bottom   [q/esc] back"
	case ScreenHelp:
		return "[j/k] scroll   [g/G] top/bottom   [esc/q/enter] close"
	default:
		return keyHints
	}
}

func (m Model) pickerBody() string {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	lines := []string{titleStyle.Render("Environments"), ""}
	for i, e := range m.envs() {
		marker := "  "
		if e.ID == m.state.ActivePrefixID {
			marker = "* "
		}
		suffix := ""
		if m.pickerLocked[e.ID] {
			suffix = "  (locked)"
		}
		count, _ := strconv.Atoi(e.Meta[ecosystem.MetaPkgCount])
		line := pickerRow(i == m.pickerCursor, marker, e.Meta[ecosystem.MetaSource], e.Rank, count, displayPath(e.ID)+suffix)
		lines = append(lines, line)
	}
	if len(m.envs()) == 0 {
		lines = append(lines, "scanning…")
	}
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// managerBody renders the package-manager switcher: exactly one manager is
// active (marked with *), and selecting another changes only the active
// manager — installed state and pending marks are preserved.
func (m Model) managerBody() string {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	ids := m.managerIDs()
	lines := []string{titleStyle.Render("Package managers"), ""}
	for i, id := range ids {
		marker := "  "
		if id == m.activeManagerID {
			marker = "* "
		}
		n := len(m.envsByManager[id])
		line := fmt.Sprintf("%s%-12s %d destination(s)", marker, id, n)
		if i == m.managerCursor {
			line = cursorStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// targetsBody renders the install-target popup: the eligible destinations of
// the active manager with multi-select markers (x = selected).
func (m Model) targetsBody() string {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	lines := []string{titleStyle.Render("Install " + m.installName + " — choose destination(s)"), ""}
	for i, e := range m.installTargets {
		marker := "  "
		if m.installTargetSel[e.ID] {
			marker = "x "
		}
		line := marker + displayPath(e.ID)
		if i == m.installTargetCursor {
			line = cursorStyle.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", hintStyle.Render("space: toggle   enter: confirm   esc/q: cancel"))
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// pickerRow renders one picker row as self-contained styled segments so the
// cursor highlight spans the whole row (a nested style's trailing reset would
// otherwise end the reverse video after the first styled cell).
func pickerRow(cursor bool, marker, source, version string, count int, path string) string {
	seg := func(s string) string { return s }
	srcS := sourceStyle
	if cursor {
		seg = func(s string) string { return cursorStyle.Render(s) }
		srcS = cursorSourceStyle
	}
	return lipgloss.JoinHorizontal(lipgloss.Left,
		seg(marker),
		padRight(srcS.Render(source), 6),
		" ",
		seg(padRight(version, 10)),
		seg(fmt.Sprintf("%3d pkgs", count)),
		"  ",
		seg(path),
	)
}

// planBody renders the plan preview: one section per (destination, manager)
// group, each listing its installs (name, target version, approximate
// download size), removals (freed space) and upgrades (from→to). Groups whose
// destination is marked under two managers are flagged invalid; operations
// involved in an unresolved conflict carry the same "!" indicator as the list.
// While the conflict gate popup is up it replaces the whole body.
func (m Model) planBody() string {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	if m.planGate {
		return m.planGateBody(bodyH)
	}
	lines := []string{titleStyle.Render("Plan"), ""}
	groups := m.planGroups()
	if len(groups) == 0 {
		lines = append(lines, "nothing to do")
	}
	mark := func(base string, dest string, p *domain.PkgState) string {
		if m.cellConflicted(dest, p.Name) {
			return lipgloss.JoinHorizontal(lipgloss.Left, base, noticeStyle.Render("  ! conflict"))
		}
		return base
	}
	for _, g := range groups {
		header := fmt.Sprintf("%s @ %s", g.manager, displayPath(g.dest))
		if g.invalid {
			header += "  — INVALID: destination is marked under two managers"
		}
		lines = append(lines, sectionStyle.Render(header))
		for _, p := range g.installs {
			v := targetVersionFor(p, g.manager)
			if v == "" {
				v = "latest"
			}
			sz := "…"
			if b, ok := m.planSizes[p.Name]; ok {
				sz = humanSize(b)
			}
			lines = append(lines, mark(fmt.Sprintf("  install %-28s %s", padRight(p.Name+"@"+v, 28), sz), g.dest, p))
		}
		for _, p := range g.removals {
			sz := "…"
			if p.SizeBytes != nil {
				sz = humanSize(*p.SizeBytes)
			}
			lines = append(lines, mark(fmt.Sprintf("  remove  %-28s frees %s", padRight(p.Name, 28), sz), g.dest, p))
		}
		for _, p := range g.upgrades {
			to := targetVersionFor(p, g.manager)
			if to == "" {
				to = "latest"
			}
			lines = append(lines, mark(fmt.Sprintf("  upgrade %-16s %s → %s", padRight(p.Name, 16), p.InstalledVersion, to), g.dest, p))
		}
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// resolverBody renders the per-package resolution screen: each conflict of
// the cell with its options (label, stated consequence, size delta where
// known), and — for redundancy conflicts — the per-destination table of
// copies with versions and sizes (spec: Node dedupe flavor reuses the
// per-destination detail view).
func (m Model) resolverBody() string {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	lines := []string{titleStyle.Render("Resolve — " + m.resolverName), ""}

	mgr := m.managerForDest(m.resolverDest)
	copies := m.dedupeCopies(mgr, m.resolverName)
	if len(copies) >= 2 {
		lines = append(lines, sectionStyle.Render("Copies by destination"))
		for _, c := range copies {
			sz := "…"
			if c.size != nil {
				sz = humanSize(*c.size)
			}
			line := fmt.Sprintf("  %-40s %-12s %s", truncate(displayPath(c.dest), 40), padRight(c.version, 12), sz)
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}

	conflicts := m.conflictsForCell(m.resolverDest, m.resolverName)
	if len(conflicts) == 0 {
		lines = append(lines, "no unresolved conflicts for this package")
	}
	flatIdx := 0
	for _, cf := range conflicts {
		lines = append(lines, sectionStyle.Render(fitText(cf.Message, m.width)))
		for _, o := range cf.Options {
			delta := ""
			if o.SizeDelta != nil {
				switch d := *o.SizeDelta; {
				case d < 0:
					delta = fmt.Sprintf(" (frees %s)", humanSize(-d))
				case d > 0:
					delta = fmt.Sprintf(" (+%s)", humanSize(d))
				default:
					delta = " (no size change)"
				}
			}
			label := fmt.Sprintf("  %2d. %-40s%s", flatIdx+1, truncate(o.Label, 40), delta)
			if flatIdx == m.resolverCursor {
				label = cursorStyle.Render(label)
			}
			lines = append(lines, label)
			lines = append(lines, "    "+fitText(o.Description, m.width-4))
			flatIdx++
		}
	}
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// planGateBody is the Yes/No popup raised when a plan contains unresolved
// conflicts (spec: plan gate on unresolved conflicts).
func (m Model) planGateBody(bodyH int) string {
	n := 0
	for _, g := range m.planGroups() {
		if g.invalid {
			continue
		}
		for _, list := range [][]*domain.PkgState{g.installs, g.upgrades, g.removals} {
			for _, p := range list {
				if m.cellConflicted(g.dest, p.Name) {
					n++
				}
			}
		}
	}
	lines := []string{
		titleStyle.Render("Plan — conflicts"),
		"",
		fmt.Sprintf("%d operation(s) in this plan are involved in unresolved conflict(s).", n),
		"",
		"There are conflicts in the plan; would you like to resolve them?",
		"",
		"  [y] open the resolver for the affected packages",
		"  [n] show the plan with the conflicting rows marked",
	}
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// infoBody renders the package detail screen (spec: package-info). Missing
// fields render as "(none)" so the layout never breaks.
func (m Model) infoBody() string {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	lines := []string{titleStyle.Render("Info — " + m.infoName), ""}

	var row *domain.PkgState
	if ps := m.state.Active(); ps != nil {
		row = ps.Packages[m.infoName]
	}
	version, tag := "", ""
	switch {
	case row != nil && row.InstalledVersion != "":
		version, tag = row.InstalledVersion, "installed"
	case row != nil && row.LatestVersion != "":
		version, tag = row.LatestVersion, "latest"
	}
	if version != "" {
		lines = append(lines, fmt.Sprintf("Version:  %s (%s)", version, tag))
	} else {
		lines = append(lines, "Version:  (none)")
	}

	if n := len(m.envs()); n > 0 {
		lines = append(lines, fmt.Sprintf("Destinations (%s) — [j/k] select, [+/-] mark:", m.activeManagerID))
		for i, e := range m.envs() {
			ver := "absent"
			flag := ""
			if ps := m.state.Prefixes[e.ID]; ps != nil {
				if p := ps.Packages[m.infoName]; p != nil {
					if p.Installed() {
						ver = p.InstalledVersion
					}
					f := p.FlagFor(m.activeManagerID)
					if f[1] != '*' {
						flag = "  [" + f + "]"
					}
				}
			}
			marker := "  "
			if i == m.infoDestCursor {
				marker = "* "
			}
			line := fmt.Sprintf("%s%s %s%s", marker, truncate(displayPath(e.ID), m.width-30), ver, flag)
			if i == m.infoDestCursor {
				line = cursorStyle.Render(line)
			}
			lines = append(lines, line)
		}
	}

	if m.infoErr != "" {
		lines = append(lines, noticeStyle.Render(m.infoErr))
	}
	if m.infoDoc == nil && m.infoErr == "" {
		lines = append(lines, "loading…")
	}
	d := m.infoDoc
	if d != nil {
		lines = append(lines, infoField("Description", d.Description))
		lines = append(lines, infoField("Homepage", d.Homepage))
		lines = append(lines, infoField("Repository", d.Repository))
		lines = append(lines, infoField("License", d.License))
		if len(d.Maintainers) > 0 {
			lines = append(lines, "Maintainers:")
			for _, mt := range d.Maintainers {
				lines = append(lines, "  "+mt)
			}
		} else {
			lines = append(lines, infoField("Maintainers", ""))
		}
		if row != nil && row.Installed() {
			if len(d.Bin) > 0 {
				lines = append(lines, "Bin:")
				for k, v := range d.Bin {
					lines = append(lines, fmt.Sprintf("  %s -> %s", k, v))
				}
			} else {
				lines = append(lines, infoField("Bin", ""))
			}
			if row.SizeBytes != nil {
				lines = append(lines, "Disk usage:  "+humanSize(*row.SizeBytes))
			} else {
				lines = append(lines, "Disk usage:  …")
			}
		}
		if len(d.Dependencies) > 0 {
			lines = append(lines, "Dependencies:")
			for _, dep := range sortedDeps(d.Dependencies) {
				lines = append(lines, fmt.Sprintf("  %-30s %s", truncate(dep.name, 30), dep.range_))
			}
		}
		if len(d.PeerDependencies) > 0 {
			lines = append(lines, "Peer dependencies:")
			for _, dep := range sortedDeps(d.PeerDependencies) {
				lines = append(lines, fmt.Sprintf("  %-30s %s", truncate(dep.name, 30), dep.range_))
			}
		}
	}
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func infoField(label, value string) string {
	if value == "" {
		value = "(none)"
	}
	return label + ":  " + value
}

type depPair struct {
	name   string
	range_ string
}

func sortedDeps(deps map[string]string) []depPair {
	out := make([]depPair, 0, len(deps))
	for k, v := range deps {
		out = append(out, depPair{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// versionsBody renders the published-version list; the installed version gets
// a "*" marker and the latest dist-tag is labeled.
func (m Model) versionsBody() string {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	lines := []string{titleStyle.Render("Versions — " + m.infoName), ""}
	versions := []string{}
	if m.infoDoc != nil {
		versions = m.infoDoc.Versions
	}
	if len(versions) == 0 {
		if m.infoErr != "" {
			lines = append(lines, noticeStyle.Render(m.infoErr))
		} else {
			lines = append(lines, "loading…")
		}
	}
	for i, v := range versions {
		marker := "  "
		if row := m.selectedInfoRow(); row != nil && v == row.InstalledVersion {
			marker = "* "
		}
		extra := ""
		if m.infoDoc != nil && v == m.infoDoc.Latest {
			extra = "  (latest)"
		}
		line := fmt.Sprintf("%s%s%s", marker, v, extra)
		if i == m.verCursor {
			line = cursorStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m Model) selectedInfoRow() *domain.PkgState {
	if ps := m.state.Active(); ps != nil {
		return ps.Packages[m.infoName]
	}
	return nil
}

// readmeContentH is the number of text lines the README body can show below
// the header and its title line.
func (m Model) readmeContentH() int {
	h := m.height - 2 - 2 // header(2) + title + blank
	if h < 1 {
		h = 1
	}
	return h
}

// readmeBody renders the full-screen scrollable README text.
func (m Model) readmeBody() string {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	lines := []string{titleStyle.Render("README — " + m.infoName), ""}
	h := m.readmeContentH()
	scroll := m.readmeScroll
	if scroll > len(m.readmeLines)-h && len(m.readmeLines) >= h {
		scroll = len(m.readmeLines) - h
	}
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + h
	if end > len(m.readmeLines) {
		end = len(m.readmeLines)
	}
	for _, l := range m.readmeLines[scroll:end] {
		lines = append(lines, fitText(l, m.width))
	}
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// helpSections is the full key-binding reference shown on the help screen.
type helpSection struct {
	title string
	rows  [][2]string
}

var helpSections = []helpSection{
	{title: "Marks", rows: [][2]string{
		{"+", "install (popup picks destinations) / upgrade to latest"},
		{"-", "remove from the headline destination"},
		{"=", "hold (excluded from bulk upgrades)"},
		{":", "revert mark on the selected row"},
		{"r", "open the resolver for a conflicted package (! marker)"},
		{"U", "mark all upgradable packages"},
		{"x", "clear all marks"},
	}},
	{title: "List", rows: [][2]string{
		{"j/k, arrows", "move cursor"},
		{"enter / d", "package info screen"},
		{"g", "open the plan preview (press g again there to apply)"},
		{"S", "cycle sort: name, version, size, state"},
		{"u", "refresh list and rescan environments"},
		{"e / E", "next environment / environment picker"},
		{"M", "package manager switcher (one active at a time)"},
	}},
	{title: "Search & filter", rows: [][2]string{
		{"/", "search the registry (esc clears; j loads more at the end)"},
		{"f", "filter expression: ~i ~u ~b ~c ~n <regex>, ! & |, parens"},
		{"l", "match installed package names"},
	}},
	{title: "Conflict resolution", rows: [][2]string{
		{"!", "row/plan op involved in an unresolved conflict (~c filters them)"},
		{"r", "resolver screen for the selected conflicted package"},
		{"j/k + enter", "choose an option (label, consequence, size delta)"},
		{"g (plan)", "gate popup when the plan has conflicts: [y] resolve / [n] marked plan"},
	}},
	{title: "Info screen", rows: [][2]string{
		{"j/k", "move the per-destination table cursor"},
		{"+", "mark install on the destination under the cursor"},
		{"-", "mark removal from the destination under the cursor"},
		{"v", "published versions (enter pins one)"},
		{"C", "README view"},
	}},
	{title: "Install targets", rows: [][2]string{
		{"space", "toggle a destination's selection"},
		{"enter", "confirm — one install mark per chosen destination"},
		{"esc/q", "cancel (no marks recorded)"},
	}},
	{title: "Plan screen", rows: [][2]string{
		{"g / enter", "apply the pending changes"},
		{"n / esc", "cancel (marks stay pending)"},
	}},
	{title: "General", rows: [][2]string{
		{"?", "this help screen"},
		{"q", "quit (confirms when marks are pending)"},
	}},
}

func helpContentLines() []string {
	lines := []string{titleStyle.Render("Help — key bindings"), ""}
	for _, s := range helpSections {
		lines = append(lines, sectionStyle.Render(s.title))
		for _, r := range s.rows {
			lines = append(lines, "  "+padRight(r[0], 16)+r[1])
		}
		lines = append(lines, "")
	}
	return lines
}

// helpBody renders the scrollable key-binding reference.
func (m Model) helpBody() string {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	all := helpContentLines()
	scroll := m.helpScroll
	if scroll > len(all)-bodyH && len(all) >= bodyH {
		scroll = len(all) - bodyH
	}
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + bodyH
	if end > len(all) {
		end = len(all)
	}
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(
		lipgloss.JoinVertical(lipgloss.Left, all[scroll:end]...))
}

var (
	linkRE = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)
	listRE = regexp.MustCompile(`^(\s*)([-*+]|\d+\.)\s+`)
)

// markdownToText is the minimal markdown-to-text pass of design D9: headings
// keep their text, lists and code fences are indented, links become
// "text (url)", emphasis markers are stripped. Failures degrade to raw lines.
func markdownToText(md string) []string {
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			out = append(out, "    "+line)
			continue
		}
		line = stripEmphasis(line)
		line = linkRE.ReplaceAllString(line, "$1 ($2)")
		if strings.HasPrefix(trimmed, "#") {
			out = append(out, strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
			continue
		}
		if loc := listRE.FindStringSubmatchIndex(line); loc != nil {
			out = append(out, line[:loc[0]]+"  "+line[loc[0]:])
			continue
		}
		out = append(out, line)
	}
	return out
}

func stripEmphasis(s string) string {
	for _, m := range []string{"**", "__", "~~", "*", "_", "`"} {
		s = strings.ReplaceAll(s, m, "")
	}
	return s
}

func (m Model) listRegion(h int) string {
	nameW := m.width - colFlag - colSize - colVer - colCand - colCount
	if nameW < 10 {
		nameW = 10
	}
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		padRight("Flag", colFlag),
		padRight("Name", nameW),
		padRight("Size", colSize),
		padRight("Version", colVer),
		padRight("Candidate", colCand),
		padRight("Also", colCount),
	)
	lines := []string{headerStyle.Render(header)}
	rows := m.displayRows()
	end := m.listTop + (h - 1)
	if end >= len(rows) {
		end = len(rows) - 1
	}
	for i := m.listTop; i <= end && len(lines) < h; i++ {
		u := rows[i]
		r := u.Row
		// Each cell is rendered as a self-contained styled segment: nesting a
		// pre-styled string inside the cursor style would let its trailing
		// reset end the reverse video after the first column. A conflicted row
		// gets the distinct "!" appended to its flag (non-intrusive: no other
		// cell changes).
		flagCell := padRight(u.Flag(m.activeManagerID), colFlag)
		flagStyleCell := flagStyle
		cursorFlagStyleCell := cursorFlagStyle
		if m.rowConflicted(u) {
			flagCell = padRight(u.Flag(m.activeManagerID)+"!", colFlag)
			flagStyleCell = conflictStyle
			cursorFlagStyleCell = conflictCursorStyle
		}
		nameCell := truncate(u.Name, nameW)
		sizeCellS := padRight(sizeCell(u.SizeBytes()), colSize)
		verCell := padRight(r.InstalledVersion, colVer)
		candCell := truncate(candidateCell(r), colCand)
		countCell := ""
		if u.Count > 0 {
			countCell = fmt.Sprintf("+%d", u.Count)
		}
		countCellS := padRight(countCell, colCount)
		var row string
		if i == m.cursor {
			row = lipgloss.JoinHorizontal(lipgloss.Left,
				cursorFlagStyleCell.Render(flagCell),
				cursorStyle.Render(nameCell),
				cursorStyle.Render(sizeCellS),
				cursorStyle.Render(verCell),
				cursorStyle.Render(candCell),
				cursorStyle.Render(countCellS),
			)
		} else {
			row = lipgloss.JoinHorizontal(lipgloss.Left,
				flagStyleCell.Render(flagCell),
				nameCell,
				sizeCellS,
				verCell,
				candCell,
				countCellS,
			)
		}
		lines = append(lines, row)
	}
	return lipgloss.NewStyle().Width(m.width).Height(h).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// applyBody renders the full-screen apply view: the raw manager output of every
// batch (auto-scrolled to the bottom) with a what-to-do-next line at the foot.
func (m Model) applyBody() string {
	bodyH := m.height - 2
	if bodyH < 4 {
		bodyH = 4
	}
	dests := []string{}
	for _, ab := range m.applyBatches {
		if !containsString(dests, ab.dest) {
			dests = append(dests, ab.dest)
		}
	}
	scope := fmt.Sprintf("%d destination(s)", len(dests))
	if len(dests) == 1 {
		scope = displayPath(dests[0])
	}
	title := "Applying changes — " + scope
	if m.applyDone {
		title = "Apply finished — " + scope
	}
	logArea := bodyH - 4 // title + blank + log + bottom line
	lines := []string{titleStyle.Render(title), ""}
	start := len(m.applyLog) - logArea
	if start < 0 {
		start = 0
	}
	for _, l := range m.applyLog[start:] {
		if strings.HasPrefix(l, "$ ") {
			lines = append(lines, sectionStyle.Render(l))
		} else if strings.HasPrefix(l, "    error:") {
			lines = append(lines, noticeStyle.Render(l))
		} else {
			lines = append(lines, l)
		}
	}
	var bottom string
	switch {
	case m.applyDone:
		bottom = "[enter] continue    [q] quit"
	case m.applyBatchIdx < len(m.applyBatches):
		bottom = noticeStyle.Render(fitText(fmt.Sprintf("applying… step %d of %d — %s   [ctrl+c] abort", m.applyBatchIdx+1, len(m.applyBatches), m.applyCurrent), m.width))
	default:
		bottom = noticeStyle.Render("re-reading package list…")
	}
	lines = append(lines, "", bottom)
	return lipgloss.NewStyle().Width(m.width).Height(bodyH).Render(
		lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func sizeCell(b *int64) string {
	if b == nil {
		return "…"
	}
	return humanSize(*b)
}

func candidateCell(r *domain.PkgState) string {
	if r.LatestVersion != "" && r.LatestVersion != r.InstalledVersion {
		return r.LatestVersion
	}
	return ""
}

func (m Model) descRegion() string {
	u := m.selectedUnified()
	if u == nil {
		return lipgloss.NewStyle().Width(m.width).Height(3).Render("")
	}
	r := u.Row
	version := r.InstalledVersion
	if version == "" {
		version = r.LatestVersion
	}
	extra := ""
	if u.Headline != nil {
		extra = "  @" + displayPath(u.HeadlineID)
	}
	line1 := lipgloss.NewStyle().MaxWidth(m.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Left,
			nameStyle.Render(r.Name), " ", version, "  ["+u.Flag(m.activeManagerID)+"]", extra))
	var lines []string
	lines = append(lines, line1)
	if r.Description != "" {
		lines = append(lines, wrapText(r.Description, m.width))
	}
	return lipgloss.NewStyle().Width(m.width).Height(3).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// wrapText wraps s to w columns on word boundaries (plain, no styling).
func wrapText(s string, w int) string {
	if w < 10 {
		w = 10
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	lines := []string{}
	cur := ""
	for _, word := range words {
		if cur == "" {
			cur = word
		} else if lipgloss.Width(cur+" "+word) <= w {
			cur += " " + word
		} else {
			lines = append(lines, cur)
			cur = word
		}
	}
	lines = append(lines, cur)
	return strings.Join(lines, "\n")
}

func (m Model) promptLine() string {
	if m.prompt != nil {
		label := "filter: "
		switch m.prompt.kind {
		case PromptLocal:
			label = "match: "
		case PromptSearch:
			label = "search: "
		}
		text := label + m.prompt.input.View()
		return lipgloss.NewStyle().Width(m.width).Render(fitText(text, m.width))
	}
	if m.applyDone {
		text := "apply finished — [Enter] return  [q] quit"
		if m.applyFailed > 0 {
			text = fmt.Sprintf("apply finished, %d operation(s) failed — [Enter] return  [q] quit", m.applyFailed)
		}
		return lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).Render(noticeStyle.Render(fitText(text, m.width)))
	}
	if m.quitConfirm {
		n := m.state.TotalPending()
		text := fmt.Sprintf("%d package(s) marked but not applied — really quit? [y] quit  [n] stay", n)
		return lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).Render(noticeStyle.Render(fitText(text, m.width)))
	}
	if m.notice != "" {
		return lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).Render(noticeStyle.Render(fitText(m.notice, m.width)))
	}
	return lipgloss.NewStyle().Width(m.width).Render("")
}

// keyHints is the list-screen action line under the title (design D8 keymap),
// truncated to fit; the full reference lives on the ? help screen.
const keyHints = "+ - = : marks  r resolve  U upgradable  x clear  enter info  / search  f filter  l match  S sort  u refresh  e/E envs  g apply  ? help  q quit"

func (m Model) statusLine() string {
	prefixID := m.state.ActivePrefixID
	rows := m.displayRows()
	total := 0
	if m.searchActive {
		if ps := m.state.Active(); ps != nil {
			total = len(ps.Packages)
		}
	} else {
		total = m.unifiedTotal()
	}
	pending := m.pendingOps()
	filterTxt := "(none)"
	if m.state.FilterText != "" {
		filterTxt = m.state.FilterText
	}
	searchTxt := ""
	if m.searchActive {
		if m.searchTotal > 0 {
			searchTxt = fmt.Sprintf(" search:%q %d/%d", m.searchQuery, m.searchFetched, m.searchTotal)
		} else {
			searchTxt = fmt.Sprintf(" search:%q", m.searchQuery)
		}
	}
	sortTxt := " sort:" + m.state.SortKey.String()
	if m.searchActive {
		sortTxt = "" // the local sort does not apply to registry results
	}
	left := fmt.Sprintf("%d/%d pkgs, %d pending%s f:%s%s mgr:%s %s",
		len(rows), total, pending, sortTxt, filterTxt, searchTxt, m.activeManagerID, displayPath(prefixID))
	return lipgloss.NewStyle().Width(m.width).Render(
		statusStyle.Render(fitText(left, m.width)))
}

// fitText shortens s with an ellipsis so it fits in w display columns.
func fitText(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	runes := []rune(s)
	for i := len(runes); i > 0; i-- {
		cut := string(runes[:i])
		if lipgloss.Width(cut+"…") <= w {
			return cut + "…"
		}
	}
	return "…"
}

func displayPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(b)/float64(div), "KMGTPE"[exp])
}

func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return padRight(s, w)
	}
	out := ""
	for _, r := range s {
		if lipgloss.Width(out+string(r)) > w-1 {
			break
		}
		out += string(r)
	}
	return out + "…"
}

func padRight(s string, w int) string {
	if lipgloss.Width(s) >= w {
		return s
	}
	return s + repeat(" ", w-lipgloss.Width(s))
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

var (
	// primaryColor fills the header band; white text on top keeps AA contrast.
	primaryColor     = lipgloss.Color("28")
	headerTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(primaryColor)
	headerHintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(primaryColor)

	headerStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
	flagStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	cursorFlagStyle   = flagStyle.Reverse(true)
	conflictStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	conflictCursorStyle = conflictStyle.Reverse(true)
	nameStyle         = lipgloss.NewStyle().Bold(true)
	cursorStyle       = lipgloss.NewStyle().Reverse(true)
	cursorSourceStyle = sourceStyle.Reverse(true)
	noticeStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	statusStyle       = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	sourceStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	sectionStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
	hintStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)
