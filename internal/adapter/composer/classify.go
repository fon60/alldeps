package composer

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/fon60/alldeps/internal/ecosystem"
)

// solverFailureMarker is the line composer prints when the dependency solver
// rejects the pending operations.
const solverFailureMarker = "Your requirements could not be resolved"

var (
	reProblem     = regexp.MustCompile(`^Problem \d+$`)
	reNotFound    = regexp.MustCompile(`(?i)could not find (?:a matching version )?of package ([\w.@/-]+)`)
	reNotFoundAlt = regexp.MustCompile(`(?i)package ([\w./-]+) could not be found`)
	rePlatform    = regexp.MustCompile(`([\w./-]+)(?:\[[^\]]*\]| [^\s]+)? requires? php (\S+) -> your php version \(([^)]+)\) does not satisfy`)
	reRootSat     = regexp.MustCompile(`^Root composer\.json requires ([\w./-]+) (\S+) -> satisfiable by [\w./-]+\[(.+)\]`)
	reRootFound   = regexp.MustCompile(`^Root composer\.json requires ([\w./-]+) (\S+), found [\w./-]+\[(.+)\] but it does not match`)
	reRootClash   = regexp.MustCompile(`^[\w./-]+\[[^\]]*\] require ([\w./-]+) (\S+) -> found [\w./-]+\[[^\]]*\] but it conflicts with your root composer\.json require \((.+)\)\.$`)
)

// Resolve probes every pending op kind through the composer solver in dry-run
// mode (design D2): a passing probe contributes its executable batch, a
// failing one is classified into conflicts with machine-applicable options and
// stops further probing. Probing never touches the project.
func (e *Ecosystem) Resolve(intent ecosystem.Intent) (ecosystem.Plan, []ecosystem.Conflict, error) {
	var batches []ecosystem.Batch
	for _, op := range []ecosystem.OpKind{ecosystem.OpInstall, ecosystem.OpUpgrade, ecosystem.OpRemove} {
		var items []ecosystem.MarkedItem
		for _, it := range intent.Items {
			if it.Op == op {
				items = append(items, it)
			}
		}
		if len(items) == 0 {
			continue
		}
		args := append(composerArgs(op, toItems(items)), "--dry-run")
		out, err := e.runComposer(args...)
		if err != nil {
			if errors.Is(err, errNoComposer) {
				return ecosystem.Plan{Env: intent.Env}, nil, err
			}
			if conflicts, ok := e.classifyFailure(op, items, out); ok {
				return ecosystem.Plan{Env: intent.Env, Batches: batches}, conflicts, nil
			}
			msg := tail(out)
			if msg == "" {
				msg = err.Error()
			}
			return ecosystem.Plan{Env: intent.Env}, nil, fmt.Errorf("composer %s failed: %s", composerArgs(op, toItems(items))[0], msg)
		}
		batches = append(batches, ecosystem.Batch{Op: op, Items: toItems(items), Label: "composer " + strings.Join(composerArgs(op, toItems(items)), " ")})
	}
	return ecosystem.Plan{Env: intent.Env, Batches: batches}, nil, nil
}

func toItems(items []ecosystem.MarkedItem) []ecosystem.Item {
	out := make([]ecosystem.Item, 0, len(items))
	for _, it := range items {
		out = append(out, ecosystem.Item{Name: it.Name, Version: it.Version})
	}
	return out
}

// classifyFailure turns one failed dry-run invocation into conflicts when the
// output is a recognizable solver or package-discovery failure; ok=false means
// the failure is not classifiable (toolchain, network, …) and must surface as
// an error instead.
func (e *Ecosystem) classifyFailure(op ecosystem.OpKind, items []ecosystem.MarkedItem, text string) ([]ecosystem.Conflict, bool) {
	if m := reNotFound.FindStringSubmatch(text); m != nil {
		name := strings.TrimRight(m[1], ".,;")
		return []ecosystem.Conflict{e.notFoundConflict(op, items, name)}, true
	}
	if strings.Contains(text, solverFailureMarker) {
		blocks := splitProblems(text)
		var conflicts []ecosystem.Conflict
		for _, block := range blocks {
			conflicts = append(conflicts, e.classifyBlock(op, items, block))
		}
		if len(conflicts) > 0 {
			return conflicts, true
		}
		return fallbackConflicts(op, items, text), true
	}
	return nil, false
}

// splitProblems cuts the solver output into Problem blocks: each block is the
// run of "- " detail lines following a "Problem N" header.
func splitProblems(text string) [][]string {
	var blocks [][]string
	var cur []string
	close := func() {
		if cur != nil {
			blocks = append(blocks, cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if reProblem.MatchString(t) {
			close()
			continue
		}
		if strings.HasPrefix(t, "- ") {
			if cur == nil {
				cur = []string{}
			}
			cur = append(cur, t[len("- "):])
			continue
		}
		if t != "" {
			close()
		}
	}
	close()
	return blocks
}

// classifyBlock classifies one Problem block (design D3): platform mismatch,
// root-dep clash / impossible constraint, package not found, or the fallback
// carrying the raw solver text.
func (e *Ecosystem) classifyBlock(op ecosystem.OpKind, items []ecosystem.MarkedItem, block []string) ecosystem.Conflict {
	text := strings.Join(block, " ")
	if m := rePlatform.FindStringSubmatch(text); m != nil {
		return e.platformConflict(op, items, text, m[1], m[2], m[3])
	}
	var candPkg, candConstraint, candidates, clashPkg, clashRequired, rootConstraint string
	for _, line := range block {
		switch m := reRootSat.FindStringSubmatch(line); {
		case m != nil:
			candPkg, candConstraint, candidates = m[1], m[2], m[3]
		default:
			if m := reRootFound.FindStringSubmatch(line); m != nil {
				candPkg, candConstraint, candidates = m[1], m[2], m[3]
			} else if m := reRootClash.FindStringSubmatch(line); m != nil {
				clashPkg, clashRequired, rootConstraint = m[1], m[2], m[3]
			}
		}
	}
	if candPkg != "" || clashPkg != "" {
		return e.rootClashConflict(op, items, text, candPkg, candConstraint, candidates, clashPkg, clashRequired, rootConstraint)
	}
	if m := reNotFoundAlt.FindStringSubmatch(text); m != nil {
		return e.notFoundConflict(op, items, m[1])
	}
	return fallbackConflict(op, items, text)
}

// platformConflict offers pinning the target's last release line that supports
// the project's PHP (from Packagist metadata) plus skipping.
func (e *Ecosystem) platformConflict(op ecosystem.OpKind, items []ecosystem.MarkedItem, blockText, pkg, constraint, yourPHP string) ecosystem.Conflict {
	if yourPHP == "" {
		yourPHP = e.phpVersion()
	}
	name := attribute(items, blockText)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	v, latestReq := e.compatibleRelease(ctx, pkg, yourPHP)
	var opts []ecosystem.ResolutionOption
	if v != "" {
		opts = append(opts, ecosystem.ResolutionOption{
			Label:       fmt.Sprintf("Install %s %s (last release supporting php %s)", pkg, v, yourPHP),
			Description: fmt.Sprintf("%s latest requires php %s; your platform is php %s", pkg, orUnknown(latestReq), yourPHP),
			Effect:      ecosystem.ResolutionEffect{Kind: effectKind(op), Name: name, TargetVersion: v},
		})
	}
	opts = append(opts, skipOption(op, name))
	return ecosystem.Conflict{Package: name, Message: trimSolverText(blockText), Options: opts}
}

// compatibleRelease walks the package's versions newest-first and returns the
// first stable release whose php requirement is satisfied by php (or carries
// none), plus the latest stable release's php requirement for the consequence.
func (e *Ecosystem) compatibleRelease(ctx context.Context, pkg, php string) (string, string) {
	vs, err := e.packagist.Metadata(ctx, pkg)
	if err != nil {
		return "", ""
	}
	var latestReq string
	for _, v := range vs {
		if isUnstable(v.Version) {
			continue
		}
		if latestReq == "" {
			latestReq = v.Require["php"]
		}
		if c, ok := v.Require["php"]; !ok || phpSatisfies(c, php) {
			return v.Version, latestReq
		}
	}
	return "", latestReq
}

// rootClashConflict offers pinning the candidates named in the solver output,
// re-pinning or removing the clashing root requirement, and skipping.
func (e *Ecosystem) rootClashConflict(op ecosystem.OpKind, items []ecosystem.MarkedItem, blockText, candPkg, candConstraint, candidates, clashPkg, clashRequired, rootConstraint string) ecosystem.Conflict {
	name := attribute(items, blockText)
	var opts []ecosystem.ResolutionOption
	if candPkg != "" && candidates != "" {
		if best := bestCandidate(candidates); best != "" {
			opts = append(opts, ecosystem.ResolutionOption{
				Label:       fmt.Sprintf("Pin %s to %s", candPkg, best),
				Description: fmt.Sprintf("require exactly %s instead of %s", best, candConstraint),
				Effect:      ecosystem.ResolutionEffect{Kind: effectKind(op), Name: candPkg, TargetVersion: best},
			})
		}
	}
	if clashPkg != "" {
		if clashRequired != "" {
			opts = append(opts, ecosystem.ResolutionOption{
				Label:       fmt.Sprintf("Pin %s to %s", clashPkg, clashRequired),
				Description: fmt.Sprintf("change the root requirement (currently %s) so both sides match", rootConstraint),
				Effect:      ecosystem.ResolutionEffect{Kind: "install", Name: clashPkg, TargetVersion: clashRequired},
			})
		}
		opts = append(opts, ecosystem.ResolutionOption{
			Label:       fmt.Sprintf("Remove %s from the project", clashPkg),
			Description: fmt.Sprintf("drop the root requirement %s that the new package conflicts with", rootConstraint),
			Effect:      ecosystem.ResolutionEffect{Kind: "remove", Name: clashPkg},
		})
	}
	opts = append(opts, skipOption(op, name))
	return ecosystem.Conflict{Package: name, Message: trimSolverText(blockText), Options: opts}
}

// notFoundConflict offers the closest Packagist match as a corrected install
// plus skipping.
func (e *Ecosystem) notFoundConflict(op ecosystem.OpKind, items []ecosystem.MarkedItem, name string) ecosystem.Conflict {
	var opts []ecosystem.ResolutionOption
	if sug := e.suggestName(name); sug != "" && sug != name {
		opts = append(opts, ecosystem.ResolutionOption{
			Label:       fmt.Sprintf("Install %s instead", sug),
			Description: fmt.Sprintf("%s was not found on Packagist; %s is the closest match", name, sug),
			Effect:      ecosystem.ResolutionEffect{Kind: "install", Name: sug},
		})
	}
	opts = append(opts, skipOption(op, name))
	return ecosystem.Conflict{Package: attribute(items, name), Message: fmt.Sprintf("package %s could not be found on Packagist", name), Options: opts}
}

// suggestName asks Packagist search for the closest match to a missing
// package name (falling back to its basename); "" when nothing is close.
func (e *Ecosystem) suggestName(name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	queries := []string{name}
	if i := strings.LastIndex(name, "/"); i >= 0 && i+1 < len(name) {
		queries = append(queries, name[i+1:])
	}
	for _, q := range queries {
		hits, _, err := e.packagist.Search(ctx, q, 1, 0)
		if err == nil && len(hits) > 0 {
			return hits[0].Name
		}
	}
	return ""
}

// fallbackConflict carries the raw solver text with generic options: choosing
// another version only records a decision (empty effect), skipping clears the
// mark.
func fallbackConflict(op ecosystem.OpKind, items []ecosystem.MarkedItem, blockText string) ecosystem.Conflict {
	name := attribute(items, blockText)
	return ecosystem.Conflict{
		Package: name,
		Message: trimSolverText(blockText),
		Options: []ecosystem.ResolutionOption{
			{Label: "Choose another version", Description: "the solver rejected this operation; pick a different version and re-resolve", Effect: ecosystem.ResolutionEffect{}},
			skipOption(op, name),
		},
	}
}

// fallbackConflicts attributes an unparseable solver failure to every pending
// item of the batch, so none of them can silently proceed.
func fallbackConflicts(op ecosystem.OpKind, items []ecosystem.MarkedItem, text string) []ecosystem.Conflict {
	var out []ecosystem.Conflict
	for _, it := range items {
		c := fallbackConflict(op, items, text)
		c.Package = it.Name
		out = append(out, c)
	}
	return out
}

// bestCandidate picks the highest stable version named in a solver candidate
// list ("v6.4.0, ..., v6.4.46"); unstable names are used only when no stable
// one is present.
func bestCandidate(list string) string {
	var best, bestUnstable string
	for _, part := range strings.Split(list, ",") {
		v := strings.TrimSpace(part)
		if i := strings.IndexByte(v, ' '); i >= 0 {
			v = v[:i] // drop annotations like "(alias of dev-master)"
		}
		if v == "" || v == "..." {
			continue
		}
		if isUnstable(v) {
			if bestUnstable == "" || compareVersions(v, bestUnstable) > 0 {
				bestUnstable = v
			}
			continue
		}
		if best == "" || compareVersions(v, best) > 0 {
			best = v
		}
	}
	if best != "" {
		return best
	}
	return bestUnstable
}

// attribute names the pending item a problem block is about: the first intent
// item mentioned in the text, else the first item.
func attribute(items []ecosystem.MarkedItem, text string) string {
	for _, it := range items {
		if it.Name != "" && strings.Contains(text, it.Name) {
			return it.Name
		}
	}
	if len(items) > 0 {
		return items[0].Name
	}
	return ""
}

func skipOption(op ecosystem.OpKind, name string) ecosystem.ResolutionOption {
	verb := "installing"
	switch op {
	case ecosystem.OpUpgrade:
		verb = "upgrading"
	case ecosystem.OpRemove:
		verb = "removing"
	}
	return ecosystem.ResolutionOption{
		Label:       fmt.Sprintf("Skip %s %s", verb, name),
		Description: fmt.Sprintf("%s will not be applied; the mark is cleared", name),
		Effect:      ecosystem.ResolutionEffect{Kind: "skip", Name: name},
	}
}

func effectKind(op ecosystem.OpKind) string {
	if op == ecosystem.OpUpgrade {
		return "upgrade"
	}
	return "install"
}

func orUnknown(s string) string {
	if s == "" {
		return "an unknown version"
	}
	return s
}

// trimSolverText condenses raw solver output for display: trimmed, capped at a
// sentence boundary when long.
func trimSolverText(text string) string {
	text = strings.TrimSpace(text)
	const cap = 500
	if len(text) <= cap {
		return text
	}
	cut := text[:cap]
	if i := strings.LastIndex(cut, " "); i > cap/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

// tail returns the last non-empty line of a command output, capped.
func tail(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			if len(l) > 300 {
				return l[:300] + "…"
			}
			return l
		}
	}
	return ""
}
