package app

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"npmitude/internal/filter"
	"npmitude/internal/state"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI256) // deterministic styling in tests
	os.Exit(m.Run())
}

func render80x24(m Model) string {
	m.width = 80
	m.height = 24
	return m.View()
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func TestHeaderShowsTitleAndScreenActions(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	out := render80x24(m)
	lines := strings.Split(out, "\n")
	wantTitle := fmt.Sprintf("npmitude %s @ %s", Version, m.hostname)
	if got := strings.TrimRight(ansiRE.ReplaceAllString(lines[0], ""), " "); got != wantTitle {
		t.Fatalf("line 1 = %q, want %q", got, wantTitle)
	}
	line2 := ansiRE.ReplaceAllString(lines[1], "")
	if !strings.HasPrefix(line2, "+ - = : marks") || !strings.Contains(line2, "f filter") {
		t.Fatalf("line 2 should list the list-screen actions:\n%s", line2)
	}

	m.screen = ScreenPlan
	out = render80x24(m)
	lines = strings.Split(out, "\n")
	if !strings.Contains(lines[1], "[g] apply") {
		t.Fatalf("plan screen header should list its actions:\n%s", lines[1])
	}
	if got := strings.TrimRight(ansiRE.ReplaceAllString(lines[0], ""), " "); got != wantTitle {
		t.Fatal("header title missing on the plan screen")
	}
}

func TestListFrameFitsTerminalHeight(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	for _, h := range []int{12, 24, 40} {
		m.width, m.height = 80, h
		lines := strings.Split(m.View(), "\n")
		if len(lines) != h {
			t.Fatalf("height %d: View returned %d lines (terminal would scroll and lose the header)", h, len(lines))
		}
		if !strings.HasPrefix(ansiRE.ReplaceAllString(lines[0], ""), "npmitude "+Version) {
			t.Fatalf("height %d: first line is not the title:\n%s", h, lines[0])
		}
	}
}

func TestHeaderBackgroundIsPrimaryColor(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	out := render80x24(m)
	lines := strings.Split(out, "\n")
	bg := fmt.Sprintf("48;5;%s", primaryColor)
	for i, l := range lines[:2] {
		if !strings.Contains(l, bg) {
			t.Fatalf("header line %d lacks the primary background (%s):\n%s", i+1, bg, l)
		}
	}
}

var sgrRE = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// reverseVideoWidth sums the display width of all text in line rendered with
// SGR 7 (reverse video). lipgloss may merge parameters (e.g. [7;38;5;214m),
// so each SGR sequence is parsed rather than matched literally.
func reverseVideoWidth(line string) int {
	width := 0
	reverse := false
	pos := 0
	for _, loc := range sgrRE.FindAllStringIndex(line, -1) {
		if reverse {
			width += lipgloss.Width(line[pos:loc[0]])
		}
		reverse = false
		for _, p := range strings.Split(line[loc[0]+2:loc[1]-1], ";") {
			if p == "7" {
				reverse = true
			}
		}
		pos = loc[1]
	}
	if reverse {
		width += lipgloss.Width(line[pos:])
	}
	return width
}

func TestCursorRowHighlightedFullWidth(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	out := render80x24(m)
	maxW := 0
	for _, l := range strings.Split(out, "\n") {
		if w := reverseVideoWidth(l); w > maxW {
			maxW = w
		}
	}
	if maxW != 80 {
		t.Fatalf("cursor highlight covers %d columns, want the full 80", maxW)
	}
}

func TestHelpScreenListsAllKeysAndReturns(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m = m.step(t, keyMsg(t, "?"))
	if m.screen != ScreenHelp {
		t.Fatalf("screen = %v, want ScreenHelp", m.screen)
	}
	out := render80x24(m)
	for _, want := range []string{
		"Help — key bindings",
		"install / upgrade to latest",
		"cycle sort: name, version, size, state",
		"j loads more at the end",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help screen missing %q", want)
		}
	}

	m = m.step(t, keyMsg(t, "G")) // scroll to the bottom sections
	out = render80x24(m)
	for _, want := range []string{
		"published versions (enter pins one)",
		"quit (confirms when marks are pending)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help screen (scrolled) missing %q", want)
		}
	}

	m = m.step(t, keyMsg(t, "esc"))
	if m.screen != ScreenList {
		t.Fatalf("screen after esc = %v, want ScreenList", m.screen)
	}
}

func TestHelpReturnsToOpeningScreen(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.screen = ScreenPicker
	m = m.step(t, keyMsg(t, "?"))
	if m.screen != ScreenHelp || m.helpFrom != ScreenPicker {
		t.Fatalf("help opened from wrong screen: %v (from %v)", m.screen, m.helpFrom)
	}
	m = m.step(t, keyMsg(t, "enter"))
	if m.screen != ScreenPicker {
		t.Fatalf("screen after closing help = %v, want ScreenPicker", m.screen)
	}
}

func TestStatusLineShowsAllItems(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	m := modelWithLoadedPrefix(t, home+"/.nvm/versions/node/v24.12.0", "alpha", "beta")
	out := render80x24(m)
	for _, want := range []string{
		"~/.nvm/versions/node/v24.12.0", // prefix path (tilde-shown)
		"f:(none)",                      // active filter
		"2/2 packages",                  // visible count
		"0 pending",                     // pending mark count
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status line missing %q in:\n%s", want, out)
		}
	}
}

func TestStatusLinePendingCountUpdates(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta", "gamma")
	out := render80x24(m)
	if !strings.Contains(out, "0 pending") {
		t.Fatalf("expected 0 pending initially:\n%s", out)
	}

	m.state.SetMark("/p", "alpha", state.MarkInstall)
	m.state.SetMark("/p", "beta", state.MarkRemove)
	out = render80x24(m)
	if !strings.Contains(out, "2 pending") {
		t.Fatalf("expected 2 pending after marking:\n%s", out)
	}

	m.state.ClearAllMarks("/p")
	out = render80x24(m)
	if !strings.Contains(out, "0 pending") {
		t.Fatalf("expected 0 pending after clearing:\n%s", out)
	}
}

func TestStatusLineShowsActiveFilter(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.FilterText = "~i & ~u"
	out := render80x24(m)
	if !strings.Contains(out, "f:~i & ~u") {
		t.Fatalf("active filter not shown:\n%s", out)
	}
}

func TestStatusLineVisibleCountRespectsFilter(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	m.state.Prefixes["/p"].Packages["alpha"].LatestVersion = "9.9.9" // upgradable
	pred, err := filter.Parse("~u")
	if err != nil {
		t.Fatal(err)
	}
	m.filterPred = pred
	out := render80x24(m)
	if !strings.Contains(out, "1/2 packages") {
		t.Fatalf("expected 1/2 visible with ~u filter:\n%s", out)
	}
}

func TestListViewportFollowsCursor(t *testing.T) {
	names := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		names = append(names, fmt.Sprintf("pkg%02d", i))
	}
	m := modelWithLoadedPrefix(t, "/p", names...)
	m.width, m.height = 80, 24 // list viewport holds 17 rows

	for i := 0; i < 20; i++ {
		m = m.step(t, keyMsg(t, "j"))
	}
	if m.cursor != 20 || m.listTop != 4 {
		t.Fatalf("after 20 downs: cursor=%d top=%d, want 20/4 (viewport must follow)", m.cursor, m.listTop)
	}
	out := render80x24(m)
	if !strings.Contains(out, "pkg04") || strings.Contains(out, "pkg00") {
		t.Fatalf("viewport must start at row 4 while the cursor is at row 20:\n%s", out)
	}

	for i := 0; i < 20; i++ {
		m = m.step(t, keyMsg(t, "k"))
	}
	if m.cursor != 0 || m.listTop != 0 {
		t.Fatalf("after scrolling back up: cursor=%d top=%d, want 0/0", m.cursor, m.listTop)
	}
	if out := render80x24(m); !strings.Contains(out, "pkg00") {
		t.Fatalf("first row must be visible again after scrolling up:\n%s", out)
	}
}

func TestHeaderBandSpansFullWidth(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	lines := strings.Split(render80x24(m), "\n")
	for i, l := range lines[:2] {
		if w := lipgloss.Width(l); w != 80 {
			t.Fatalf("header line %d is %d columns wide, want full 80:\n%s", i+1, w, l)
		}
	}
}
