// Package npmcmd runs the target prefix's own node + npm and parses its
// stable JSON outputs (design D2/D3). Prose is never parsed.
package npmcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"npmitude/internal/registry"
	"npmitude/internal/sizes"
)

// ParsedPkg is one top-level global package as reported by npm ls.
type ParsedPkg struct {
	Name    string
	Version string
	Broken  bool // subtree contains missing/invalid dependencies
}

// invalidFlag accepts npm's per-node "invalid" field. It is a boolean for
// real problems (conflicting peer dependencies) but a reason string for
// optional dependencies skipped on platform mismatch — those are expected
// and must not mark the package as broken. Skipped nodes also carry a
// self-referential "problems" entry, which callers must ignore via Skipped.
type invalidFlag struct {
	problem bool // boolean true: real problem
	skipped bool // reason string: optional dep skipped on platform mismatch
}

func (f *invalidFlag) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		f.skipped = true
		return nil
	}
	var v bool
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	f.problem = v
	return nil
}

type lsNode struct {
	Version  string             `json:"version"`
	Missing  bool               `json:"missing"`
	Invalid  invalidFlag        `json:"invalid"`
	Problems []string           `json:"problems"`
	Deps     map[string]*lsNode `json:"dependencies"`
}

type lsOutput struct {
	Name         string             `json:"name"`
	Dependencies map[string]*lsNode `json:"dependencies"`
	Problems     []string           `json:"problems"`
	Error        *lsError           `json:"error"`
}

type lsError struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}

func (n *lsNode) subtreeBroken() bool {
	if n == nil {
		return false
	}
	if n.Missing || n.Invalid.problem {
		return true
	}
	if !n.Invalid.skipped && len(n.Problems) > 0 {
		return true // skipped nodes' problems entry is just the skip notice
	}
	for _, d := range n.Deps {
		if d.subtreeBroken() {
			return true
		}
	}
	return false
}

// ParseLS parses the JSON output of `npm ls -g --all --json` into top-level
// packages with broken flags derived from subtree validity.
func ParseLS(data []byte) (map[string]ParsedPkg, error) {
	var out lsOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse npm ls output: %w", err)
	}
	pkgs := make(map[string]ParsedPkg, len(out.Dependencies))
	for name, node := range out.Dependencies {
		if node == nil {
			continue
		}
		pkgs[name] = ParsedPkg{
			Name:    name,
			Version: node.Version,
			Broken:  node.subtreeBroken(),
		}
	}
	return pkgs, nil
}

// NodeAndNPM returns the absolute paths of a prefix's own node binary and npm
// CLI entry point.
func NodeAndNPM(prefixID string) (node, npmCLI string) {
	node = filepath.Join(prefixID, "bin", "node")
	npmCLI = filepath.Join(prefixID, "lib", "node_modules", "npm", "bin", "npm-cli.js")
	return
}

// RunNPMJSON runs a command via the prefix's own node + npm and returns
// stdout. A non-zero exit (e.g. ELSPROBLEMS) is not an error as long as
// stdout was produced; only spawn/execution failures are.
func RunNPMJSON(ctx context.Context, prefixID string, args ...string) ([]byte, error) {
	return runNPMJSON(ctx, prefixID, "", args...)
}

// RunNPMJSONIn is RunNPMJSON with the npm process rooted at workdir, so
// project-scoped commands (no -g) operate on that directory's tree.
func RunNPMJSONIn(ctx context.Context, prefixID, workdir string, args ...string) ([]byte, error) {
	return runNPMJSON(ctx, prefixID, workdir, args...)
}

func runNPMJSON(ctx context.Context, prefixID, workdir string, args ...string) ([]byte, error) {
	node, npmCLI := NodeAndNPM(prefixID)
	cmd := exec.CommandContext(ctx, node, append([]string{npmCLI}, args...)...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && stdout.Len() > 0 {
			return stdout.Bytes(), nil
		}
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("npm %v: %v: %s", args, err, bytes.TrimSpace(stderr.Bytes()))
		}
		return nil, fmt.Errorf("npm %v: %w", args, err)
	}
	return stdout.Bytes(), nil
}

// LSGlobal lists the top-level global packages of a prefix using its own npm.
func LSGlobal(ctx context.Context, prefixID string) (map[string]ParsedPkg, error) {
	data, err := RunNPMJSON(ctx, prefixID, "ls", "-g", "--all", "--json")
	if err != nil {
		return nil, err
	}
	return ParseLS(data)
}

// LSProject lists the top-level dependencies of a project directory using the
// prefix's own npm rooted there (no -g).
func LSProject(ctx context.Context, prefixID, root string) (map[string]ParsedPkg, error) {
	data, err := RunNPMJSONIn(ctx, prefixID, root, "ls", "--all", "--json")
	if err != nil {
		return nil, err
	}
	return ParseLS(data)
}

// LocalDoc reads an installed package's own package.json into a registry.Doc
// so the info screen works offline (versions/latest/readme stay empty).
func LocalDoc(prefixID, name string) (*registry.Doc, error) {
	return LocalDocAt(sizes.GlobalModuleDir(prefixID), name)
}

// LocalDocAt is LocalDoc addressed at an explicit module directory (the
// node_modules dir itself), used for project-scoped environments.
func LocalDocAt(moduleDir, name string) (*registry.Doc, error) {
	data, err := os.ReadFile(filepath.Join(moduleDir, name, "package.json"))
	if err != nil {
		return nil, err
	}
	var rd registry.RawDoc
	if err := json.Unmarshal(data, &rd); err != nil {
		return nil, fmt.Errorf("parse %s/package.json: %w", name, err)
	}
	doc := registry.NormalizeDoc(&rd)
	if doc.Name == "" {
		doc.Name = name
	}
	return doc, nil
}

// Readme reads the installed package's local README file (first candidate
// that exists), if any.
func Readme(prefixID, name string) (string, bool) {
	return ReadmeAt(sizes.GlobalModuleDir(prefixID), name)
}

// ReadmeAt is Readme addressed at an explicit module directory.
func ReadmeAt(moduleDir, name string) (string, bool) {
	dir := filepath.Join(moduleDir, name)
	for _, f := range []string{"README.md", "readme.md", "README.markdown", "README.rst", "README.txt", "README"} {
		if b, err := os.ReadFile(filepath.Join(dir, f)); err == nil {
			return string(b), true
		}
	}
	return "", false
}

// GetRegistry asks the prefix's own npm for its configured registry URL
// (design D3: ask npm itself rather than re-implementing .npmrc resolution).
func GetRegistry(ctx context.Context, prefixID string) (string, error) {
	return getRegistry(ctx, prefixID, "")
}

// GetRegistryIn is GetRegistry with npm rooted at workdir, so a project's own
// .npmrc takes effect.
func GetRegistryIn(ctx context.Context, prefixID, workdir string) (string, error) {
	return getRegistry(ctx, prefixID, workdir)
}

func getRegistry(ctx context.Context, prefixID, workdir string) (string, error) {
	data, err := RunNPMJSONIn(ctx, prefixID, workdir, "config", "get", "registry")
	if err != nil {
		return "", err
	}
	url := strings.TrimSpace(string(data))
	if url == "" {
		return "", fmt.Errorf("empty registry URL from npm config")
	}
	return url, nil
}
