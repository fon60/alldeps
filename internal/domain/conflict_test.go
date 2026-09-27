package domain

import (
	"testing"

	"github.com/fon60/alldeps/internal/ecosystem"
)

func conflictStateFixture() *PrefixState {
	return &PrefixState{
		ID: "/p",
		Packages: map[string]*PkgState{
			"beta": {Name: "beta", InstalledVersion: "1.0.0", Origin: OriginInstalled},
		},
		Loaded: true,
	}
}

func TestConflictStateSurfacesReportedConflict(t *testing.T) {
	ps := conflictStateFixture()
	c := ecosystem.Conflict{Package: "beta", Message: "beta clashes", Options: []ecosystem.ResolutionOption{{Label: "skip"}}}
	ps.Conflicts = map[string][]ecosystem.Conflict{"beta": {c}}

	if got := ps.UnresolvedConflicts("beta"); len(got) != 1 || got[0].Message != "beta clashes" {
		t.Fatalf("UnresolvedConflicts(beta) = %+v, want the reported conflict", got)
	}
	if got := ps.UnresolvedConflicts("gamma"); len(got) != 0 {
		t.Fatalf("UnresolvedConflicts(gamma) = %+v, want none", got)
	}
}

func TestChosenResolutionSuppressesUntilMarksChange(t *testing.T) {
	ps := conflictStateFixture()
	ps.Conflicts = map[string][]ecosystem.Conflict{"beta": {{Package: "beta", Message: "clash"}}}
	ps.Packages["beta"].SetMarkFor("npm", MarkRemove)

	ps.RecordResolution("beta", "skip installing beta")
	if got := ps.UnresolvedConflicts("beta"); len(got) != 0 {
		t.Fatalf("conflict still unresolved after a matching pick: %+v", got)
	}
	if !ps.ResolutionChosen("beta") {
		t.Fatal("ResolutionChosen must be true while the cell's marks are unchanged")
	}

	ps.Packages["beta"].RevertFor("npm")
	if ps.ResolutionChosen("beta") {
		t.Fatal("clearing the cell's mark must invalidate the pick")
	}
	if got := ps.UnresolvedConflicts("beta"); len(got) != 1 {
		t.Fatalf("conflict must be unresolved again after the mark changed: %+v", got)
	}

	ps.RecordResolution("beta", "skip installing beta")
	if got := ps.UnresolvedConflicts("beta"); len(got) != 0 {
		t.Fatalf("re-pick on the unchanged cell must suppress again: %+v", got)
	}
	ps.ForgetResolution("beta")
	if got := ps.UnresolvedConflicts("beta"); len(got) != 1 {
		t.Fatalf("ForgetResolution must re-open the conflict: %+v", got)
	}
}

func TestCellMarkSigTracksAllManagers(t *testing.T) {
	ps := conflictStateFixture()
	if sig := ps.CellMarkSig("beta"); sig != "" {
		t.Fatalf("empty cell signature = %q, want empty", sig)
	}
	ps.Packages["beta"].SetMarkFor("npm", MarkRemove)
	first := ps.CellMarkSig("beta")
	if first == "" {
		t.Fatal("signature must be non-empty with a pending mark")
	}
	ps.Packages["beta"].SetMarkEntry("yarn", MarkEntry{Mark: MarkInstall, TargetVersion: "2.0.0"})
	if sig := ps.CellMarkSig("beta"); sig == first {
		t.Fatal("adding a second manager's mark must change the signature")
	}
	ps.Packages["beta"].SetMarkEntry("yarn", MarkEntry{Mark: MarkInstall, TargetVersion: "3.0.0"})
	if sig := ps.CellMarkSig("beta"); sig == first {
		t.Fatal("changing a target version must change the signature")
	}
}
