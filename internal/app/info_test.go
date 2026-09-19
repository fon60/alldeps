package app

import (
	"strings"
	"testing"

	"npmitude/internal/domain"
	"npmitude/internal/ecosystem"
)

func ptr[T any](v T) *T { return &v }

func TestInfoScreenInstalledUsesLocalDoc(t *testing.T) {
	s := newStubEco()
	s.docs = map[string]*ecosystem.Doc{
		"foo": {
			Name:             "foo",
			Description:      "A test pkg",
			License:          "MIT",
			Bin:              map[string]string{"foo": "cli.js"},
			Dependencies:     map[string]string{"bar": "^1.0.0"},
			PeerDependencies: map[string]string{"baz": "*"},
		},
	}
	m := modelWithLoadedPrefix(t, "/p", "foo")
	m.eco = s

	nextRaw, cmd := m.Update(keyMsg(t, "enter"))
	if cmd == nil {
		t.Fatal("opening info should start a metadata load")
	}
	m = nextRaw.(Model)
	msg := cmd()
	m = m.step(t, msg)

	if m.screen != ScreenInfo {
		t.Fatalf("screen = %v, want info", m.screen)
	}
	if !m.infoLocal {
		t.Fatal("installed package must use the local doc")
	}
	if m.infoDoc == nil || m.infoDoc.Description != "A test pkg" {
		t.Fatalf("doc = %+v", m.infoDoc)
	}
	if m.infoDoc.Bin["foo"] != "cli.js" {
		t.Fatalf("bin = %v (string form must be keyed by name)", m.infoDoc.Bin)
	}
	if m.infoDoc.Dependencies["bar"] != "^1.0.0" || m.infoDoc.PeerDependencies["baz"] != "*" {
		t.Fatal("dependency sections missing")
	}

	m = m.step(t, keyMsg(t, "esc"))
	if m.screen != ScreenList {
		t.Fatal("esc should return to the list")
	}
}

func TestInfoScreenOfflineNotInstalledShowsNotice(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "ghost")
	m.state.Prefixes["/p"].Packages["ghost"].InstalledVersion = ""
	m.state.Prefixes["/p"].Packages["ghost"].Origin = domain.OriginSearch

	_, cmd := m.Update(keyMsg(t, "d"))
	m2 := m.step(t, keyMsg(t, "d"))
	msg := cmd()
	m2 = m2.step(t, msg)

	if m2.screen != ScreenInfo {
		t.Fatalf("screen = %v, want info", m2.screen)
	}
	if !strings.Contains(m2.infoErr, "unavailable") {
		t.Fatalf("infoErr = %q, want an unavailability notice", m2.infoErr)
	}
	if m2.infoDoc != nil {
		t.Fatal("no doc should be fabricated on fetch failure")
	}
}

func TestPinVersionInstalledAndNotInstalled(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha", "beta")
	m.state.Prefixes["/p"].Packages["beta"].InstalledVersion = ""
	m.state.Prefixes["/p"].Packages["beta"].Origin = domain.OriginSearch

	m.screen = ScreenInfo
	m.infoName = "alpha"
	m.infoDoc = &ecosystem.Doc{Name: "alpha", Versions: []string{"2.0.0", "1.5.0", "1.0.0"}, Latest: "2.0.0"}
	m.screen = ScreenVersions
	m.verCursor = 1 // 1.5.0

	m = m.step(t, keyMsg(t, "enter"))
	a := m.state.Prefixes["/p"].Packages["alpha"]
	if a.Mark != domain.MarkUpgrade || a.TargetVersion != "1.5.0" {
		t.Fatalf("installed pin: mark=%v target=%q", a.Mark, a.TargetVersion)
	}
	if m.screen != ScreenInfo {
		t.Fatal("pin should return to the info screen")
	}

	m.screen = ScreenVersions
	m.infoName = "beta"
	m.verCursor = 0 // 2.0.0
	m = m.step(t, keyMsg(t, "enter"))
	b := m.state.Prefixes["/p"].Packages["beta"]
	if b.Mark != domain.MarkInstall || b.TargetVersion != "2.0.0" {
		t.Fatalf("not-installed pin: mark=%v target=%q", b.Mark, b.TargetVersion)
	}
}

func TestQuitConfirmationProtectsMarks(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	m.state.SetMark("/p", "alpha", domain.MarkRemove)

	m = m.step(t, keyMsg(t, "q"))
	if !m.quitConfirm {
		t.Fatal("quit with pending marks must ask for confirmation")
	}
	m = m.step(t, keyMsg(t, "n"))
	if m.quitConfirm {
		t.Fatal("cancel should return to the list")
	}

	m = m.step(t, keyMsg(t, "q"))
	nextRaw, cmd := m.Update(keyMsg(t, "y"))
	_ = nextRaw
	if cmd == nil {
		t.Fatal("confirming the prompt should quit")
	}
}

func TestQuitWithoutMarksQuitsImmediately(t *testing.T) {
	m := modelWithLoadedPrefix(t, "/p", "alpha")
	_, cmd := m.Update(keyMsg(t, "q"))
	if cmd == nil {
		t.Fatal("quit without pending marks must not prompt")
	}
}

func TestMarkdownToText(t *testing.T) {
	md := "# Title\n\nSome **bold** and `code` text.\n- one\n- two\n1. three\n\n```\ncode line\n```\n\n[link](https://x.io)\n"
	lines := strings.Join(markdownToText(md), "\n")
	for _, want := range []string{"Title", "bold and code text", "  - one", "  1. three", "    code line", "link (https://x.io)"} {
		if !strings.Contains(lines, want) {
			t.Errorf("markdownToText missing %q in:\n%s", want, lines)
		}
	}
	for _, bad := range []string{"**", "`", "[link]("} {
		if strings.Contains(lines, bad) {
			t.Errorf("markdownToText left artifact %q in:\n%s", bad, lines)
		}
	}
}

func TestInfoViewRendersFieldsAndAbsence(t *testing.T) {
	s := newStubEco()
	s.docs = map[string]*ecosystem.Doc{
		"foo": {
			Name:             "foo",
			License:          "MIT",
			Bin:              map[string]string{"foo": "cli.js", "bar": "other.js"},
			Dependencies:     map[string]string{"dep-a": "^1.0.0"},
			PeerDependencies: map[string]string{"peer-b": "*"},
		},
	}
	m := modelWithLoadedPrefix(t, "/p", "foo")
	m.eco = s
	m.state.Prefixes["/p"].Packages["foo"].SizeBytes = ptr(int64(2048))

	nextRaw, cmd := m.Update(keyMsg(t, "enter"))
	m = nextRaw.(Model)
	m = m.step(t, cmd())

	m.width, m.height = 80, 24
	v := m.View()
	for _, want := range []string{
		"Info — foo",
		"Version:  1.0.0 (installed)",
		"Description:  (none)", // absent field renders as missing
		"License:  MIT",
		"Disk usage:  2.0K",
		"Bin:", "foo -> cli.js", "bar -> other.js",
		"Dependencies:", "  dep-a", "^1.0.0",
		"Peer dependencies:", "  peer-b",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("info view missing %q in:\n%s", want, v)
		}
	}
	for _, bad := range []string{"(none)\r", "**"} {
		if strings.Contains(v, bad) {
			t.Errorf("info view contains artifact %q", bad)
		}
	}
}

func TestReadmeViewLocalAndAbsent(t *testing.T) {
	s := newStubEco()
	s.readmes = map[string]string{"foo": "# foo\nreadme body"}
	m := modelWithLoadedPrefix(t, "/p", "foo")
	m.eco = s
	m.screen = ScreenInfo
	m.infoName = "foo"

	m = m.step(t, keyMsg(t, "C"))
	if m.screen != ScreenReadme {
		t.Fatalf("screen = %v, want readme", m.screen)
	}
	joined := strings.Join(m.readmeLines, "\n")
	if !strings.Contains(joined, "readme body") {
		t.Fatalf("readme lines = %q", joined)
	}

	m2 := modelWithLoadedPrefix(t, "/p", "bar")
	m2.screen = ScreenInfo
	m2.infoName = "bar"
	m2.state.Prefixes["/p"].Packages["bar"].InstalledVersion = ""
	m2.state.Prefixes["/p"].Packages["bar"].Origin = domain.OriginSearch
	// no registry configured -> the fetch fails with ErrNoRegistry and lands
	// on the absent notice
	nextRaw, cmd := m2.Update(keyMsg(t, "C"))
	if cmd == nil {
		t.Fatal("opening the readme for an uninstalled package must start a fetch")
	}
	m2 = nextRaw.(Model)
	m2 = m2.step(t, cmd())
	if !strings.Contains(strings.Join(m2.readmeLines, "\n"), "no README available") {
		t.Fatalf("readme lines = %q, want absent notice", m2.readmeLines)
	}
}
