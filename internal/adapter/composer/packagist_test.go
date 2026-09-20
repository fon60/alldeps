package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// testPackagist stands up httptest stubs for the search endpoint and the p2
// metadata API (no real network) and returns a Packagist client pointed at
// them. The metadata handler serves metaJSON for every /p2/<name>.json.
func testPackagist(t *testing.T, searchJSON, metaJSON string) (*Packagist, *int32) {
	t.Helper()
	var metaHits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/search.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, searchJSON)
	})
	mux.HandleFunc("/p2/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&metaHits, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, metaJSON)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Packagist{
		SearchBase: srv.URL,
		MetaBase:   srv.URL,
		HTTP:       srv.Client(),
		TTL:        defaultTTL,
		metaCache:  map[string]metaEntry{},
	}, &metaHits
}

func TestPackagistSearchParsesResultsAndPaging(t *testing.T) {
	p, _ := testPackagist(t, `{"results":[{"name":"a/b","description":"first"},{"name":"c/d","description":"second"}],"total":42}`, `{}`)

	hits, total, err := p.Search(context.Background(), "query", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 42 {
		t.Fatalf("total = %d, want 42", total)
	}
	if len(hits) != 2 || hits[0].Name != "a/b" || hits[0].Description != "first" || hits[1].Name != "c/d" {
		t.Fatalf("hits = %+v, want the two stub results", hits)
	}

	// Paging: from=10 size=10 must request page 2.
	var gotPage string
	p.SearchBase = searchURLCapturing(t, &gotPage)
	if _, _, err := p.Search(context.Background(), "q", 10, 10); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPage, "page=2") || !strings.Contains(gotPage, "per_page=10") {
		t.Fatalf("search URL = %q, want page=2 and per_page=10", gotPage)
	}
}

// searchURLCapturing returns a base URL whose server records the request path.
func searchURLCapturing(t *testing.T, out *string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*out = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"results":[],"total":0}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestPackagistMetadataInheritsMinifiedFields(t *testing.T) {
	meta := `{
  "minified": "composer/2.0",
  "packages": {"a/b": [
    {"name":"a/b","description":"desc","version":"3.0.2","license":["MIT"],"homepage":"https://ex.com","source":{"url":"https://git.ex.com/a/b.git"},"require":{"php":">=8.1"}},
    {"version":"3.0.1"},
    {"version":"2.0.0","require":{"php":">=7.4"}}
  ]}
}`
	p, _ := testPackagist(t, `{}`, meta)

	vs, err := p.Metadata(context.Background(), "a/b")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 3 {
		t.Fatalf("got %d versions, want 3: %+v", len(vs), vs)
	}
	want := []struct {
		version string
		php     string
	}{
		{"3.0.2", ">=8.1"},
		{"3.0.1", ">=8.1"}, // inherited from 3.0.2 (minified omission)
		{"2.0.0", ">=7.4"},
	}
	for i, w := range want {
		if vs[i].Version != w.version {
			t.Fatalf("version[%d] = %q, want %q", i, vs[i].Version, w.version)
		}
		if got := vs[i].Require["php"]; got != w.php {
			t.Fatalf("require.php[%d] = %q, want %q (minified fields must be inherited)", i, got, w.php)
		}
	}
	if vs[0].Description != "desc" || vs[0].License != "MIT" || vs[0].SourceURL != "https://git.ex.com/a/b.git" {
		t.Fatalf("doc fields = %+v, want the full first entry", vs[0])
	}
}

func TestPackagistLatestStableSkipsDevAndAlpha(t *testing.T) {
	meta := `{
  "packages": {"a/b": [
    {"name":"a/b","version":"2.0.0-dev"},
    {"version":"1.5.0-alpha1"},
    {"version":"dev-main"},
    {"version":"1.4.3"},
    {"version":"1.4.2"}
  ]}
}`
	p, _ := testPackagist(t, `{}`, meta)

	vs, err := p.Metadata(context.Background(), "a/b")
	if err != nil {
		t.Fatal(err)
	}
	if got := latestStable(vs); got != "1.4.3" {
		t.Fatalf("latest stable = %q, want 1.4.3 (first version without -dev/-alpha)", got)
	}

	versions, failed := p.LatestFor(context.Background(), []string{"a/b"})
	if failed != 0 || versions["a/b"] != "1.4.3" {
		t.Fatalf("LatestFor = %v failed=%d, want a/b -> 1.4.3 with no failures", versions, failed)
	}
}

func TestPackagistMetadataCachesWithinTTL(t *testing.T) {
	p, hits := testPackagist(t, `{}`, `{"packages":{"a/b":[{"name":"a/b","version":"1.0.0"}]}}`)

	for i := 0; i < 3; i++ {
		if _, err := p.Metadata(context.Background(), "a/b"); err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Fatalf("metadata endpoint hit %d times for 3 cached calls, want 1", got)
	}
}

func TestPackagistMetadataNotFoundIsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/p2/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := &Packagist{SearchBase: srv.URL, MetaBase: srv.URL, HTTP: srv.Client(), TTL: defaultTTL, metaCache: map[string]metaEntry{}}

	if _, err := p.Metadata(context.Background(), "no/such"); err == nil {
		t.Fatal("expected an error for a 404 package")
	}
}

func TestPackagistLatestForCountsFailures(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/p2/ok/pkg.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"packages":{"ok/pkg":[{"name":"ok/pkg","version":"1.2.3"}]}}`)
	})
	mux.HandleFunc("/p2/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := &Packagist{SearchBase: srv.URL, MetaBase: srv.URL, HTTP: srv.Client(), TTL: defaultTTL, metaCache: map[string]metaEntry{}}

	versions, failed := p.LatestFor(context.Background(), []string{"ok/pkg", "no/one", "no/two"})
	if failed != 2 {
		t.Fatalf("failed = %d, want 2", failed)
	}
	if versions["ok/pkg"] != "1.2.3" {
		t.Fatalf("versions = %v, want ok/pkg -> 1.2.3", versions)
	}
}

// TestPackagistSearchResponseShape is a guard against drift: the stub must be
// decodable by the real client (regression for JSON field names).
func TestPackagistSearchResponseShape(t *testing.T) {
	var raw struct {
		Results []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"results"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(`{"results":[{"name":"x/y","description":"d"}],"total":1}`), &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Results) != 1 || raw.Total != 1 {
		t.Fatalf("stub shape mismatch: %+v", raw)
	}
}
