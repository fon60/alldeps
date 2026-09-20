// Package composer implements the ecosystem.Ecosystem port for Composer (PHP)
// projects: listing from vendor/composer/installed.json, Packagist search and
// latest-stable lookup, operations through `composer require/remove` with
// lifecycle scripts enabled, and conflict detection through --dry-run solver
// probes whose failures are classified into resolution options.
package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultSearchBase = "https://packagist.org"
	DefaultMetaBase   = "https://repo.packagist.org"
)

// Packagist is a client for the Packagist index: the search endpoint and the
// p2 package-metadata API. Metadata results are TTL-cached per package name.
type Packagist struct {
	SearchBase string
	MetaBase   string
	HTTP       *http.Client
	TTL        time.Duration

	mu        sync.Mutex
	metaCache map[string]metaEntry
}

type metaEntry struct {
	versions  []PkgVersion
	fetchedAt time.Time
}

// PkgVersion is one version of a package from p2 metadata, with fields the
// minified format omits already inherited from the newer entries.
type PkgVersion struct {
	Version     string
	Require     map[string]string
	Description string
	License     string
	Homepage    string
	SourceURL   string
}

// NewPackagist returns a client for the public Packagist endpoints.
func NewPackagist() *Packagist {
	return &Packagist{
		SearchBase: DefaultSearchBase,
		MetaBase:   DefaultMetaBase,
		HTTP:       &http.Client{Timeout: 30 * time.Second},
		TTL:        defaultTTL,
		metaCache:  map[string]metaEntry{},
	}
}

type searchHit struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type searchResponse struct {
	Results []searchHit `json:"results"`
	Total   int         `json:"total"`
}

// Search queries the Packagist search endpoint. Results carry no version (the
// caller fills it from Metadata); paging is page-based, with from an offset.
func (p *Packagist) Search(ctx context.Context, query string, size, from int) ([]searchHit, int, error) {
	q := url.Values{}
	q.Set("q", query)
	if size > 0 {
		q.Set("per_page", fmt.Sprintf("%d", size))
	}
	if from > 0 && size > 0 {
		q.Set("page", fmt.Sprintf("%d", from/size+1))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(p.SearchBase, "/")+"/search.json?"+q.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("packagist search: %s", resp.Status)
	}
	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, 0, fmt.Errorf("packagist search: parse: %w", err)
	}
	return sr.Results, sr.Total, nil
}

type p2Response struct {
	Packages map[string][]map[string]any `json:"packages"`
}

// Metadata fetches the p2 metadata of one package (vendor/name), returning
// its versions newest-first with minified fields inherited across entries.
func (p *Packagist) Metadata(ctx context.Context, name string) ([]PkgVersion, error) {
	p.mu.Lock()
	if e, ok := p.metaCache[name]; ok && time.Since(e.fetchedAt) < p.TTL {
		p.mu.Unlock()
		return e.versions, nil
	}
	p.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(p.MetaBase, "/")+"/p2/"+name+".json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("packagist: package %s not found", name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("packagist: %s for %s", resp.Status, name)
	}
	var pr p2Response
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("packagist: parse %s: %w", name, err)
	}
	entries, ok := pr.Packages[name]
	if !ok {
		for _, v := range pr.Packages {
			entries = v
			break
		}
	}
	out := make([]PkgVersion, 0, len(entries))
	var inherited map[string]any
	for _, raw := range entries {
		eff := map[string]any{}
		for k, v := range inherited {
			eff[k] = v
		}
		for k, v := range raw {
			eff[k] = v
		}
		inherited = eff
		out = append(out, PkgVersion{
			Version:     strOf(eff["version"]),
			Require:     requireMap(eff["require"]),
			Description: strOf(eff["description"]),
			License:     firstLicense(eff["license"]),
			Homepage:    strOf(eff["homepage"]),
			SourceURL:   sourceURL(eff["source"]),
		})
	}
	p.mu.Lock()
	p.metaCache[name] = metaEntry{versions: out, fetchedAt: time.Now()}
	p.mu.Unlock()
	return out, nil
}

// LatestFor resolves the latest stable version of every name in parallel with
// bounded concurrency, returning the resolved map and the failure count.
func (p *Packagist) LatestFor(ctx context.Context, names []string) (map[string]string, int) {
	results := make(map[string]string, len(names))
	var (
		mu     sync.Mutex
		failed int
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
			vs, err := p.Metadata(ctx, name)
			if err != nil {
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			v := latestStable(vs)
			mu.Lock()
			defer mu.Unlock()
			if v == "" {
				failed++
				return
			}
			results[name] = v
		}()
	}
	wg.Wait()
	return results, failed
}

// latestStable returns the first version that is not a dev/alpha release (the
// p2 order is newest-first); "" when there is none.
func latestStable(vs []PkgVersion) string {
	for _, v := range vs {
		if !isUnstable(v.Version) {
			return v.Version
		}
	}
	return ""
}

func isUnstable(v string) bool {
	v = strings.ToLower(v)
	return strings.HasPrefix(v, "dev-") || strings.HasSuffix(v, "-dev") || strings.Contains(v, "-alpha")
}

func strOf(v any) string {
	s, _ := v.(string)
	return s
}

func requireMap(v any) map[string]string {
	m, _ := v.(map[string]any)
	out := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}

func firstLicense(v any) string {
	switch l := v.(type) {
	case string:
		return l
	case []any:
		if len(l) > 0 {
			return strOf(l[0])
		}
	}
	return ""
}

func sourceURL(v any) string {
	m, _ := v.(map[string]any)
	return strOf(m["url"])
}
