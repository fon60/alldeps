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
// LSGlobal lists exactly its globals.
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

	gotNames := make([]string, 0, len(got))
	for name, p := range got {
		gotNames = append(gotNames, name)
		want, ok := ref.Dependencies[name]
		if !ok {
			t.Errorf("unexpected package %q", name)
			continue
		}
		if p.Version != want.Version {
			t.Errorf("%s: version %q, want %q", name, p.Version, want.Version)
		}
	}
	sort.Strings(gotNames)
	refNames := make([]string, 0, len(ref.Dependencies))
	for name := range ref.Dependencies {
		refNames = append(refNames, name)
	}
	sort.Strings(refNames)
	if len(gotNames) != len(refNames) {
		t.Fatalf("LSGlobal found %v, reference has %v", gotNames, refNames)
	}
	for i := range gotNames {
		if gotNames[i] != refNames[i] {
			t.Fatalf("package set mismatch: %v vs %v", gotNames, refNames)
		}
	}
	t.Logf("prefix %s: %d globals match exactly: %v", prefixID, len(gotNames), gotNames)
}
