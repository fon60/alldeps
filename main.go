package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"npmitude/internal/adapter/npm"
	"npmitude/internal/app"
	"npmitude/internal/ecosystem"
)

func main() {
	arg := ""
	if len(os.Args) > 1 {
		arg = os.Args[1]
	}
	mode, root, err := app.SelectMode(arg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "npmitude:", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p := tea.NewProgram(buildModel(ctx, mode, root))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// buildModel wires the launch: global mode around the npm adapter, project
// mode around the adapters detected as applicable to root (each bound to its
// own project instance).
func buildModel(ctx context.Context, mode app.Mode, root string) app.Model {
	if mode == app.ModeGlobal {
		return app.New(npm.New())
	}
	adapters := []ecosystem.Ecosystem{npm.New()}
	applicable := app.DetectApplicable(adapters, root)
	managers := make(map[string]ecosystem.Ecosystem, len(applicable))
	ids := make([]string, 0, len(applicable))
	for _, a := range applicable {
		// Node-family variants are all served by the npm adapter bound to the
		// project; other adapters (when they exist) serve themselves.
		if _, ok := a.Adapter.(*npm.Ecosystem); ok {
			managers[a.ID] = npm.NewProject(ctx, root)
		} else {
			managers[a.ID] = a.Adapter
		}
		ids = append(ids, a.ID)
	}
	return app.NewProject(root, managers, ids)
}
