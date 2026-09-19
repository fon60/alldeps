package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestQuitKey(t *testing.T) {
	m := New(newStubEco())
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if _, ok := next.(Model); !ok {
		t.Fatal("expected Model back from Update")
	}
	if cmd == nil {
		t.Fatal("expected a quit command on q")
	}
}
