package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"npmitude/internal/app"
)

func main() {
	p := tea.NewProgram(app.New())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
