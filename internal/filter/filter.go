// Package filter implements the aptitude-subset filter language (design D5):
// ~i, ~u, ~b, ~n <pattern>, !, &, | with a small recursive-descent parser.
// Invalid expressions are rejected at parse time so the caller can retain the
// previously active filter.
package filter

import (
	"fmt"
	"regexp"
	"strings"
)

// PkgView is the minimal per-package view a predicate evaluates against.
type PkgView struct {
	Name       string
	Installed  bool
	Upgradable bool
	Broken     bool
}

// Predicate evaluates one package row.
type Predicate func(PkgView) bool

// Parse parses an expression and returns its predicate. An empty expression
// matches everything (no filter).
func Parse(expr string) (Predicate, error) {
	if strings.TrimSpace(expr) == "" {
		return func(PkgView) bool { return true }, nil
	}
	p := &parser{src: expr}
	node, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("unexpected %q at position %d", tokenAt(p.src, p.pos), p.pos)
	}
	return node.eval, nil
}

type node struct {
	eval Predicate
}

func (n *node) and(other *node) *node {
	a, b := n.eval, other.eval
	return &node{eval: func(v PkgView) bool { return a(v) && b(v) }}
}

func (n *node) or(other *node) *node {
	a, b := n.eval, other.eval
	return &node{eval: func(v PkgView) bool { return a(v) || b(v) }}
}

func (n *node) not() *node {
	a := n.eval
	return &node{eval: func(v PkgView) bool { return !a(v) }}
}

type parser struct {
	src string
	pos int
}

func tokenAt(s string, pos int) string {
	if pos >= len(s) {
		return "end of input"
	}
	c := s[pos]
	if c == '~' || c == '!' || c == '&' || c == '|' || c == '(' || c == ')' {
		return string(c)
	}
	end := pos + 1
	for end < len(s) && s[end] != ' ' && s[end] != '\t' && s[end] != ')' {
		end++
	}
	return s[pos:end]
}

// parseExpr: term ('|' term)*
func (p *parser) parseExpr() (*node, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for p.skipWS(); ; {
		if !p.at('|') {
			break
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = left.or(right)
	}
	return left, nil
}

// parseTerm: factor ('&' factor)*
func (p *parser) parseTerm() (*node, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for p.skipWS(); ; {
		if !p.at('&') {
			break
		}
		p.pos++
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = left.and(right)
	}
	return left, nil
}

// parseFactor: '!' factor | primary
func (p *parser) parseFactor() (*node, error) {
	p.skipWS()
	if p.at('!') {
		p.pos++
		inner, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		return inner.not(), nil
	}
	return p.parsePrimary()
}

// parsePrimary: '~i' | '~u' | '~b' | '~n' pattern | '(' expr ')'
func (p *parser) parsePrimary() (*node, error) {
	p.skipWS()
	if p.at('(') {
		p.pos++
		n, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.skipWS()
		if !p.at(')') {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return n, nil
	}
	if p.at('~') {
		if len(p.src) < p.pos+2 || p.src[p.pos+1] == ' ' {
			return nil, fmt.Errorf("incomplete test at position %d", p.pos)
		}
		op := p.src[p.pos : p.pos+2]
		switch op {
		case "~i":
			p.pos += 2
			return &node{eval: func(v PkgView) bool { return v.Installed }}, nil
		case "~u":
			p.pos += 2
			return &node{eval: func(v PkgView) bool { return v.Upgradable }}, nil
		case "~b":
			p.pos += 2
			return &node{eval: func(v PkgView) bool { return v.Broken }}, nil
		case "~n":
			p.pos += 2
			return p.parseNamePattern()
		default:
			return nil, fmt.Errorf("unknown test %q", op)
		}
	}
	return nil, fmt.Errorf("unexpected %q at position %d", tokenAt(p.src, p.pos), p.pos)
}

func (p *parser) parseNamePattern() (*node, error) {
	p.skipWS()
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == ' ' || c == '\t' || c == ')' || c == '|' || c == '&' {
			break
		}
		p.pos++
	}
	pat := p.src[start:p.pos]
	if pat == "" {
		return nil, fmt.Errorf("~n requires a name pattern")
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("invalid name pattern %q: %v", pat, err)
	}
	return &node{eval: func(v PkgView) bool { return re.MatchString(v.Name) }}, nil
}

func (p *parser) skipWS() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *parser) at(c byte) bool {
	return p.pos < len(p.src) && p.src[p.pos] == c
}

// View adapts a package row to the filter view. The adapter lives here so the
// filter package stays independent of the state package.
func View(name string, installed, upgradable, broken bool) PkgView {
	return PkgView{Name: name, Installed: installed, Upgradable: upgradable, Broken: broken}
}

// Apply returns the rows matching pred (all rows when pred is nil).
func Apply(rows []PkgView, pred Predicate) []PkgView {
	if pred == nil {
		return rows
	}
	kept := make([]PkgView, 0, len(rows))
	for _, r := range rows {
		if pred(r) {
			kept = append(kept, r)
		}
	}
	return kept
}
