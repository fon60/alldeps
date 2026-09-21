// Package ecosystem defines the adapter port every package-manager backend
// implements (design D2) plus the wire types that cross it. It is a boundary
// artifact: adapters implement it, the app consumes it, and neither leaks
// manager-specific knowledge past it.
package ecosystem

import (
	"context"
	"errors"
)

// ErrNoRegistry is returned by adapter read operations when the environment
// has no usable registry configured (offline or broken manager config).
var ErrNoRegistry = errors.New("no registry configured for this prefix")

// Meta is free-form adapter-supplied metadata.
type Meta map[string]string

// Well-known Environment.Meta keys populated by adapters.
const (
	MetaSource   = "source"    // discovery origin label, e.g. "nvm", "fnm", "system"
	MetaPkgCount = "pkg_count" // top-level package count at discovery time
	MetaActive   = "active"    // "1" when the environment is the manager's active one
)

// MetaVariant is the DetectProject meta key carrying the tool variant a
// project uses (for the Node family: "npm", "yarn", or "pnpm"). When present,
// it becomes the applicable manager id instead of the adapter's own id.
const MetaVariant = "variant"

// Environment is a concrete destination this manager can operate on. The ID
// is opaque and adapter-defined (for npm it is the absolute Node prefix
// path); Rank is a comparable ordering key used for display/ordering; Meta
// carries adapter-supplied facts the app may show.
type Environment struct {
	ID   string
	Kind string // destination kind label, e.g. "node-prefix"
	Rank string
	Meta Meta
}

// Package is one installed package as physically present in a destination.
type Package struct {
	Name      string
	Version   string
	Unhealthy bool // generic health flag (e.g. broken dependency tree)
}

// Hit is one search result from the ecosystem's package index.
type Hit struct {
	Name        string
	Version     string
	Description string
}

// OpKind is a manager-agnostic operation class.
type OpKind int

const (
	OpInstall OpKind = iota
	OpUpgrade
	OpRemove
)

// Item is one package operand of an operation; Version "" means latest.
type Item struct {
	Name    string
	Version string
}

// MarkedItem is one user-marked operation in an intent.
type MarkedItem struct {
	Op      OpKind
	Name    string
	Version string // pinned target for install/upgrade; "" = latest
}

// Intent is the manager-agnostic description of the operations the user
// marked, addressed to one destination.
type Intent struct {
	Env   Environment
	Items []MarkedItem
}

// Batch is one invocation of the environment's own manager. Label is the
// human-readable command line shown in logs (e.g. "npm i -g foo@1.2.3").
type Batch struct {
	Op    OpKind
	Items []Item
	Label string
}

// Plan is a concrete, executable grouping of an intent's operations.
type Plan struct {
	Env     Environment
	Batches []Batch
}

// ResolutionEffect is the machine-applicable outcome of choosing an option:
// which marks it sets where. An empty Kind means the option only records a
// decision (e.g. "keep as is") and changes no marks.
type ResolutionEffect struct {
	Kind          string   // "install" | "upgrade" | "remove" | "skip"
	Name          string   // package to act on; "" = the conflict's package
	TargetVersion string   // pinned target for install/upgrade; "" = latest
	Destinations  []string // destinations to act on; empty = the conflict's own
}

// ResolutionOption is one way a user can resolve a Conflict. Description is
// the stated consequence shown before the user commits; SizeDelta is an
// approximate disk change in bytes (negative frees space, nil = unknown).
type ResolutionOption struct {
	Label       string
	Description string
	SizeDelta   *int64
	Effect      ResolutionEffect
}

// Conflict is one clash the resolver could not fold into a plan; it carries
// the options a later UI change can offer for resolving it.
type Conflict struct {
	Package string
	Message string
	Options []ResolutionOption
}

// Caps advertises what an ecosystem can do so the app can adapt without
// type-switching on the manager id. HasDedupe marks managers whose installed
// copies can be redundant across destinations (the Node family), so the app
// can derive align/consolidate/remove conflicts from its own list state.
// HasSearch marks ecosystems with a queryable package index; when false the
// app declines registry search with a notice instead of a failing query.
type Caps struct {
	HasConflictResolution bool
	HasNativeLock         bool
	GlobalScope           bool
	ProjectScope          bool
	HasDedupe             bool
	HasSearch             bool
}

// Doc is the manager-agnostic package document shown on the info screen,
// version history, and README view. UnpackedSizes maps a published version to
// its registry-reported on-disk size in bytes (npm only; empty elsewhere).
type Doc struct {
	Name             string
	Description      string
	Homepage         string
	Repository       string
	License          string
	Maintainers      []string // "name (email)"
	Bin              map[string]string
	Dependencies     map[string]string
	PeerDependencies map[string]string
	Versions         []string
	Latest           string
	Readme           string
	UnpackedSizes    map[string]int64
}

// LockHandle is a held environment lock.
type LockHandle interface {
	Release()
}

// Ecosystem is the adapter port: discover destinations, list what is
// installed, search, resolve an intent into a concrete plan (or conflicts),
// execute it with raw passthrough, lock, and report capabilities.
type Ecosystem interface {
	ID() string // stable manager id, e.g. "npm"

	Discover(ctx context.Context) ([]Environment, error)
	ListInstalled(ctx context.Context, env Environment) ([]Package, error)
	Search(ctx context.Context, env Environment, query string, size, from int) ([]Hit, int, error)
	Resolve(intent Intent) (Plan, []Conflict, error)
	Execute(ctx context.Context, env Environment, batch Batch) (string, error)
	Lock(env Environment) (LockHandle, error) // native-preferred; falls back to session lock
	DetectProject(root string) (bool, Meta)
	Capabilities() Caps

	// Read helpers for the info/versions/readme screens and upgradability
	// checks; they resolve manager-specific configuration internally.
	LatestVersions(ctx context.Context, env Environment, names []string) (map[string]string, int, error)
	UnpackedSize(ctx context.Context, env Environment, name, version string) (int64, error)
	Doc(ctx context.Context, env Environment, name string, installed bool) (*Doc, bool, error)
	Readme(env Environment, name string) (string, bool)
	Writable(env Environment) bool
}
