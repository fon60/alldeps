// Package registry talks to the configured npm registry over plain HTTP
// (design D2): search endpoint, package documents, and per-package latest
// dist-tag lookups with a TTL cache and bounded parallelism.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultTTL = 5 * time.Minute

// Client is a registry client bound to one base URL (the active prefix's
// configured registry).
type Client struct {
	BaseURL string
	HTTP    *http.Client
	TTL     time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	version   string
	fetchedAt time.Time
}

// NewClient returns a client for the given registry base URL. A trailing
// slash is tolerated and normalized away.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		TTL:     defaultTTL,
		cache:   map[string]cacheEntry{},
	}
}

// pkgURL builds the package document URL for a (possibly scoped) name,
// encoding the scope as %40 per npm registry convention.
func (c *Client) pkgURL(name string) string {
	esc := strings.ReplaceAll(url.PathEscape(name), "@", "%40")
	return c.BaseURL + "/" + esc
}

type abbreviatedDoc struct {
	DistTags map[string]string `json:"dist-tags"`
}

// LatestVersion fetches the latest dist-tag version for a package, using the
// TTL cache. The abbreviated document (install-v1) is requested to keep
// payloads small.
func (c *Client) LatestVersion(ctx context.Context, name string) (string, error) {
	c.mu.Lock()
	if e, ok := c.cache[name]; ok && time.Since(e.fetchedAt) < c.TTL {
		c.mu.Unlock()
		return e.version, nil
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.pkgURL(name), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry: %s for %s", resp.Status, name)
	}
	var doc abbreviatedDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("registry: parse %s: %w", name, err)
	}
	v := doc.DistTags["latest"]
	if v == "" {
		return "", fmt.Errorf("registry: no latest dist-tag for %s", name)
	}
	c.mu.Lock()
	c.cache[name] = cacheEntry{version: v, fetchedAt: time.Now()}
	c.mu.Unlock()
	return v, nil
}

// CheckOutdated fetches latest versions for all names in parallel with a
// bounded worker pool. It returns the resolved name->version map and the
// number of packages that failed (network error, 404, missing tag).
func (c *Client) CheckOutdated(ctx context.Context, names []string) (map[string]string, int) {
	results := make(map[string]string, len(names))
	var (
		mu     sync.Mutex
		failed int32
	)
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, name := range names {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			v, err := c.LatestVersion(ctx, name)
			if err != nil {
				atomic.AddInt32(&failed, 1)
				return
			}
			mu.Lock()
			results[name] = v
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results, int(failed)
}

type versionDoc struct {
	Versions map[string]struct {
		Dist struct {
			UnpackedSize int64 `json:"unpackedSize"`
		} `json:"dist"`
	} `json:"versions"`
}

// UnpackedSize fetches the approximate on-disk size in bytes of one package
// version from the registry's abbreviated document.
func (c *Client) UnpackedSize(ctx context.Context, name, version string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.pkgURL(name), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("registry: %s for %s", resp.Status, name)
	}
	var doc versionDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return 0, fmt.Errorf("registry: parse %s: %w", name, err)
	}
	v, ok := doc.Versions[version]
	if !ok || v.Dist.UnpackedSize <= 0 {
		return 0, fmt.Errorf("registry: no unpacked size for %s@%s", name, version)
	}
	return v.Dist.UnpackedSize, nil
}

// Doc is the normalized package document used by the info screen, version
// history, and README view. It can be filled from the registry (full) or from
// a locally installed package.json (no versions/latest/readme).
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
	UnpackedSizes    map[string]int64 // version -> registry-reported on-disk bytes
}

type RawDoc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	Repository  any    `json:"repository"` // string or {url: ...}
	License     any    `json:"license"`    // string or {type: ...}
	Maintainers []struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"maintainers"`
	Bin              any               `json:"bin"` // string or map[string]string
	Dependencies     map[string]string `json:"dependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
	Versions         map[string]struct {
		Dependencies     map[string]string `json:"dependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
		Bin              any               `json:"bin"`
		Dist             struct {
			UnpackedSize int64 `json:"unpackedSize"`
		} `json:"dist"`
	} `json:"versions"`
	DistTags map[string]string `json:"dist-tags"`
	Readme   string            `json:"readme"`
}

// GetDoc fetches the full package document from the registry.
func (c *Client) GetDoc(ctx context.Context, name string) (*Doc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.pkgURL(name), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry: %s for %s", resp.Status, name)
	}
	var rd RawDoc
	if err := json.Unmarshal(body, &rd); err != nil {
		return nil, fmt.Errorf("registry: parse %s: %w", name, err)
	}
	doc := NormalizeDoc(&rd)
	if doc.Name == "" {
		doc.Name = name
	}
	// Registry documents carry dependencies/peerDependencies/bin per version,
	// not at the top level: overlay the latest version's fields.
	if v := rd.DistTags["latest"]; v != "" {
		if ver, ok := rd.Versions[v]; ok {
			if len(doc.Dependencies) == 0 {
				doc.Dependencies = ver.Dependencies
			}
			if len(doc.PeerDependencies) == 0 {
				doc.PeerDependencies = ver.PeerDependencies
			}
			if len(doc.Bin) == 0 {
				switch b := ver.Bin.(type) {
				case string:
					if doc.Name != "" {
						doc.Bin[doc.Name] = b
					}
				case map[string]any:
					for k, val := range b {
						if s, ok := val.(string); ok {
							doc.Bin[k] = s
						}
					}
				}
			}
		}
	}
	return doc, nil
}

func NormalizeDoc(rd *RawDoc) *Doc {
	doc := &Doc{
		Name:             rd.Name,
		Description:      rd.Description,
		Homepage:         rd.Homepage,
		Bin:              map[string]string{},
		Dependencies:     rd.Dependencies,
		PeerDependencies: rd.PeerDependencies,
		Readme:           rd.Readme,
	}
	switch r := rd.Repository.(type) {
	case string:
		doc.Repository = r
	case map[string]any:
		if u, ok := r["url"].(string); ok {
			doc.Repository = u
		}
	}
	switch l := rd.License.(type) {
	case string:
		doc.License = l
	case map[string]any:
		if t, ok := l["type"].(string); ok {
			doc.License = t
		}
	}
	for _, m := range rd.Maintainers {
		if m.Email != "" {
			doc.Maintainers = append(doc.Maintainers, fmt.Sprintf("%s (%s)", m.Name, m.Email))
		} else {
			doc.Maintainers = append(doc.Maintainers, m.Name)
		}
	}
	switch b := rd.Bin.(type) {
	case string:
		if doc.Name != "" {
			doc.Bin[doc.Name] = b
		}
	case map[string]any:
		for k, v := range b {
			if s, ok := v.(string); ok {
				doc.Bin[k] = s
			}
		}
	}
	for v, ver := range rd.Versions {
		doc.Versions = append(doc.Versions, v)
		if ver.Dist.UnpackedSize > 0 {
			if doc.UnpackedSizes == nil {
				doc.UnpackedSizes = map[string]int64{}
			}
			doc.UnpackedSizes[v] = ver.Dist.UnpackedSize
		}
	}
	doc.Latest = rd.DistTags["latest"]
	return doc
}

// SearchHit is one result from the registry search endpoint.
type SearchHit struct {
	Name        string
	Version     string
	Description string
}

type searchResponse struct {
	Objects []struct {
		Package struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Description string `json:"description"`
		} `json:"package"`
	} `json:"objects"`
	Total int `json:"total"`
}

// SearchURL builds the search endpoint URL for a query (exported shape for
// testability of URL construction against custom registries).
func SearchURL(baseURL, text string, size, from int) string {
	q := url.Values{}
	q.Set("text", text)
	if size > 0 {
		q.Set("size", fmt.Sprintf("%d", size))
	}
	if from > 0 {
		q.Set("from", fmt.Sprintf("%d", from))
	}
	return strings.TrimSuffix(baseURL, "/") + "/-/v1/search?" + q.Encode()
}

// Search queries the registry search endpoint and returns one bounded page of
// results plus the total number of matches reported by the registry.
func (c *Client) Search(ctx context.Context, text string, size, from int) ([]SearchHit, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SearchURL(c.BaseURL, text, size, from), nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("registry search: %s", resp.Status)
	}
	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, 0, fmt.Errorf("registry search: parse: %w", err)
	}
	hits := make([]SearchHit, 0, len(sr.Objects))
	for _, o := range sr.Objects {
		hits = append(hits, SearchHit{
			Name:        o.Package.Name,
			Version:     o.Package.Version,
			Description: o.Package.Description,
		})
	}
	return hits, sr.Total, nil
}
