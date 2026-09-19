package ecosystem

import "testing"

// trivialResolver satisfies the port with the default Resolve path: any
// intent resolves to a plan with no conflicts.
type trivialResolver struct {
	Ecosystem // nil; only Resolve is exercised here
}

func (trivialResolver) Resolve(intent Intent) (Plan, []Conflict, error) {
	return Plan{Env: intent.Env}, nil, nil
}

func TestResolveContractReturnsEmptyConflictSet(t *testing.T) {
	var eco Ecosystem = trivialResolver{}
	intent := Intent{
		Env:   Environment{ID: "/p"},
		Items: []MarkedItem{{Op: OpInstall, Name: "foo"}},
	}
	plan, conflicts, err := eco.Resolve(intent)
	if err != nil {
		t.Fatalf("conflict-free intent must not error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflict-free intent must yield an empty conflict set, got %d", len(conflicts))
	}
	if plan.Env.ID != "/p" {
		t.Fatalf("plan env = %+v, want the intent's environment", plan.Env)
	}
}
