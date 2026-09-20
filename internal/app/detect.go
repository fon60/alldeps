package app

import (
	"sort"

	"npmitude/internal/ecosystem"
)

// Applicable is one package manager that applies to a project, as reported by
// an adapter's DetectProject. ID is the manager id the user switches between:
// the Node-family lockfile variant when the adapter reports one, else the
// adapter's own id. Meta carries the adapter's detection facts (variant, …).
type Applicable struct {
	ID      string
	Adapter ecosystem.Ecosystem
	Meta    ecosystem.Meta
}

// DetectApplicable runs the startup detection pass (design D2): it asks every
// ProjectScope-capable adapter whether it applies to root and collects the
// applicable managers in stable id order. Adapters without the capability are
// not consulted. Detection is a pure filesystem read per adapter.
func DetectApplicable(adapters []ecosystem.Ecosystem, root string) []Applicable {
	var out []Applicable
	for _, a := range adapters {
		if !a.Capabilities().ProjectScope {
			continue
		}
		ok, meta := a.DetectProject(root)
		if !ok {
			continue
		}
		id := a.ID()
		if v := meta[ecosystem.MetaVariant]; v != "" {
			id = v
		}
		out = append(out, Applicable{ID: id, Adapter: a, Meta: meta})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
