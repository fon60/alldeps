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

// Plan groups pending operations by destination.
type Plan struct {
	Ops []Op
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
