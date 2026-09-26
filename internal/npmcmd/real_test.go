package npmcmd

import (
	"context"
	"encoding/json"
	"os/exec"
	"sort"
	"testing"
	"time"
)

type refLS struct {
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
}

// TestLSGlobalRealPrefix verifies against the machine's active npm prefix that
// LSGlobal lists every top-level global (direct) plus its transitive
// dependencies (automatic), with versions matching the top-level reference.
func TestLSGlobalRealPrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	out, err := exec.Command("npm", "config", "get", "prefix").Output()
	if err != nil {
		t.Skipf("cannot determine active prefix: %v", err)
	}
	prefixID := string(out[:len(out)-1])

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	got, err := LSGlobal(ctx, prefixID)
	if err != nil {
		t.Fatalf("LSGlobal(%s): %v", prefixID, err)
	}

	refOut, err := exec.Command("npm", "ls", "-g", "--json").Output()
	if err != nil {
		t.Fatalf("reference npm ls: %v", err)
	}
	var ref refLS
	if err := json.Unmarshal(refOut, &ref); err != nil {
		t.Fatalf("parse reference: %v", err)
	}

	for name, want := range ref.Dependencies {
		p, ok := got[name]
		if !ok {
			t.Errorf("top-level global %q missing from LSGlobal", name)
			continue
		}
		if p.Automatic {
			t.Errorf("%s: top-level global must be direct, not automatic", name)
		}
		if p.Version != want.Version {
			t.Errorf("%s: version %q, want %q", name, p.Version, want.Version)
		}
	}
	for name, p := range got {
		if _, ok := ref.Dependencies[name]; !ok && !p.Automatic {
			t.Errorf("non-top-level package %q must be marked automatic", name)
		}
	}
	gotNames := make([]string, 0, len(got))
	for name := range got {
		gotNames = append(gotNames, name)
	}
	sort.Strings(gotNames)
	t.Logf("prefix %s: %d top-level globals, %d total rows: %v", prefixID, len(ref.Dependencies), len(gotNames), gotNames)
}
