package domain

import "sort"

// Op is one pending operation: act on Name at Destination via Manager.
type Op struct {
	Destination string
	Manager     string // ecosystem id performing the operation
	Name        string
	Mark        Mark
	Version     string // pinned target for install/upgrade; "" = latest
}

// Plan is the ordered set of all pending operations.
type Plan struct {
	Ops []Op
}

// Group is one (destination, manager) group: every operation that one manager
// performs on one destination in a single apply run.
type Group struct {
	Destination string
	Manager     string
	Ops         []Op
}

// Groups splits the plan's operations into (destination, manager) groups,
// ordered by destination then manager (both ascending), regardless of the
// input order.
func (p *Plan) Groups() []Group {
	ops := make([]Op, len(p.Ops))
	copy(ops, p.Ops)
	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].Destination != ops[j].Destination {
			return ops[i].Destination < ops[j].Destination
		}
		if ops[i].Manager != ops[j].Manager {
			return ops[i].Manager < ops[j].Manager
		}
		return ops[i].Name < ops[j].Name
	})
	var out []Group
	for _, op := range ops {
		if n := len(out); n > 0 && out[n-1].Destination == op.Destination && out[n-1].Manager == op.Manager {
			out[n-1].Ops = append(out[n-1].Ops, op)
			continue
		}
		out = append(out, Group{Destination: op.Destination, Manager: op.Manager, Ops: []Op{op}})
	}
	return out
}

// InvalidDestinations returns the destinations that violate the
// one-manager-per-destination invariant: within a single plan, one
// destination may be operated on by at most one manager. Distinct
// destinations are always compatible, even across managers.
func (p *Plan) InvalidDestinations() []string {
	managers := map[string]map[string]bool{}
	for _, op := range p.Ops {
		if managers[op.Destination] == nil {
			managers[op.Destination] = map[string]bool{}
		}
		managers[op.Destination][op.Manager] = true
	}
	var out []string
	for dest, ms := range managers {
		if len(ms) > 1 {
			out = append(out, dest)
		}
	}
	sort.Strings(out)
	return out
}

// Valid reports whether the plan satisfies the one-manager-per-destination
// invariant.
func (p *Plan) Valid() bool { return len(p.InvalidDestinations()) == 0 }
