package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stubRegistry serves abbreviated docs for the given name->latest map and
// counts requests. Unknown names get a 404.
func stubRegistry(t *testing.T, latest map[string]string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		path := strings.TrimPrefix(r.URL.Path, "/")
		name, err := unescapePkg(path)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		v, ok := latest[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"name":%q,"dist-tags":{"latest":%q},"versions":{"%s":{"version":"%s"}}}`, name, v, v, v)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func unescapePkg(p string) (string, error) {
	return url.PathUnescape(p)
}

func TestLatestVersionBasic(t *testing.T) {
	srv, _ := stubRegistry(t, map[string]string{"alpha": "1.4.0"})
	c := NewClient(srv.URL)
	v, err := c.LatestVersion(context.Background(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if v != "1.4.0" {
		t.Fatalf("latest = %q, want 1.4.0", v)
	}
}

func TestLatestVersionTTLCache(t *testing.T) {
	srv, hits := stubRegistry(t, map[string]string{"alpha": "1.4.0"})
	c := NewClient(srv.URL)
	ctx := context.Background()

	if _, err := c.LatestVersion(ctx, "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LatestVersion(ctx, "alpha"); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Fatalf("server hits = %d, want 1 (TTL cache should serve the second call)", got)
	}

	c.TTL = -time.Second // force expiry
	if _, err := c.LatestVersion(ctx, "alpha"); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(hits); got != 2 {
		t.Fatalf("server hits = %d, want 2 after TTL expiry", got)
	}
}

func TestLatestVersionNotFound(t *testing.T) {
	srv, _ := stubRegistry(t, map[string]string{"alpha": "1.0.0"})
	c := NewClient(srv.URL)
	if _, err := c.LatestVersion(context.Background(), "ghost"); err == nil {
		t.Fatal("expected error for unknown package")
	}
}

func TestCheckOutdatedParallelAndFailures(t *testing.T) {
	srv, _ := stubRegistry(t, map[string]string{
		"alpha": "1.4.0", "beta": "2.0.0", "gamma": "3.1.0",
	})
	c := NewClient(srv.URL)
	versions, failed := c.CheckOutdated(context.Background(), []string{"alpha", "beta", "gamma", "ghost"})
	if len(versions) != 3 {
		t.Fatalf("resolved %d versions, want 3: %v", len(versions), versions)
	}
	if versions["alpha"] != "1.4.0" || versions["beta"] != "2.0.0" || versions["gamma"] != "3.1.0" {
		t.Fatalf("wrong versions: %v", versions)
	}
	if failed != 1 {
		t.Fatalf("failed = %d, want 1 (ghost)", failed)
	}
}

func TestCheckOutdatedOffline(t *testing.T) {
	c := NewClient("http://127.0.0.1:1") // nothing listens here
	c.HTTP.Timeout = 500 * time.Millisecond
	versions, failed := c.CheckOutdated(context.Background(), []string{"alpha", "beta"})
	if len(versions) != 0 || failed != 2 {
		t.Fatalf("offline: versions=%v failed=%d, want none/2", versions, failed)
	}
}

func TestSearchURLConstruction(t *testing.T) {
	got := SearchURL("https://my.registry.example.com/custom", "package manager", 20, 0)
	want := "https://my.registry.example.com/custom/-/v1/search?size=20&text=package+manager"
	if got != want {
		t.Fatalf("SearchURL = %q, want %q", got, want)
	}
	got = SearchURL("https://r.example.org/", "x", 0, 0)
	want = "https://r.example.org/-/v1/search?text=x"
	if got != want {
		t.Fatalf("SearchURL (no size) = %q, want %q", got, want)
	}
	got = SearchURL("https://r.example.org/", "x", 20, 40)
	want = "https://r.example.org/-/v1/search?from=40&size=20&text=x"
	if got != want {
		t.Fatalf("SearchURL (paged) = %q, want %q", got, want)
	}
}

func TestScopedPackageURL(t *testing.T) {
	var wirePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wirePath = r.URL.EscapedPath()
		fmt.Fprint(w, `{"name":"@scope/pkg","dist-tags":{"latest":"1.0.0"},"versions":{}}`)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)
	if _, err := c.LatestVersion(context.Background(), "@scope/pkg"); err != nil {
		t.Fatal(err)
	}
	if wirePath != "/%40scope%2Fpkg" {
		t.Fatalf("wire path = %q, want /%%40scope%%2Fpkg", wirePath)
	}
}

func TestSearch(t *testing.T) {
	var gotFrom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/-/v1/search" || !strings.Contains(r.URL.RawQuery, "text=foo") {
			t.Errorf("unexpected request %q", r.URL.String())
		}
		gotFrom = r.URL.Query().Get("from")
		fmt.Fprint(w, `{"objects":[
			{"package":{"name":"foo","version":"1.0.0","description":"does foo"}},
			{"package":{"name":"bar","version":"2.0.0","description":""}}
		],"time":1,"total":42}`)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)
	hits, total, err := c.Search(context.Background(), "foo", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Name != "foo" || hits[0].Version != "1.0.0" || hits[0].Description != "does foo" {
		t.Fatalf("hits = %+v", hits)
	}
	if total != 42 {
		t.Fatalf("total = %d, want 42", total)
	}
	if gotFrom != "" {
		t.Fatalf("first page must not send from, got %q", gotFrom)
	}

	hits, total, err = c.Search(context.Background(), "foo", 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || total != 42 {
		t.Fatalf("second page: hits=%v total=%d", hits, total)
	}
	if gotFrom != "20" {
		t.Fatalf("second page from = %q, want 20", gotFrom)
	}
}

func TestGetDocNormalizes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"name":"foo",
			"description":"does foo",
			"homepage":"https://foo.example",
			"repository":{"type":"git","url":"git+https://github.com/x/foo.git"},
			"license":{"type":"MIT"},
			"maintainers":[{"name":"alice","email":"a@x.io"},{"name":"bob"}],
			"bin":"cli.js",
			"dependencies":{"bar":"^1.0.0"},
			"peerDependencies":{"baz":">=2"},
			"dist-tags":{"latest":"2.0.0"},
			"versions":{"1.0.0":{},"2.0.0":{"dependencies":{"qux":"~3"},"peerDependencies":{"quux":">=1"},"bin":"run.js"}},
			"readme":"# foo\nhello"
		}`)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)
	doc, err := c.GetDoc(context.Background(), "foo")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Repository != "git+https://github.com/x/foo.git" {
		t.Errorf("repository = %q (want object url extracted)", doc.Repository)
	}
	if doc.License != "MIT" {
		t.Errorf("license = %q (want object type extracted)", doc.License)
	}
	if len(doc.Maintainers) != 2 || doc.Maintainers[0] != "alice (a@x.io)" || doc.Maintainers[1] != "bob" {
		t.Errorf("maintainers = %v", doc.Maintainers)
	}
	if doc.Bin["foo"] != "cli.js" {
		t.Errorf("bin = %v (want string form keyed by name)", doc.Bin)
	}
	if len(doc.Versions) != 2 || doc.Latest != "2.0.0" {
		t.Errorf("versions/latest = %v / %q", doc.Versions, doc.Latest)
	}
	if doc.Dependencies["bar"] != "^1.0.0" || doc.PeerDependencies["baz"] != ">=2" {
		t.Errorf("deps = %v / %v", doc.Dependencies, doc.PeerDependencies)
	}
	if !strings.Contains(doc.Readme, "hello") {
		t.Errorf("readme = %q", doc.Readme)
	}
}

func TestGetDocStringForms(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"name":"bar",
			"repository":"https://bar.example/repo",
			"license":"ISC",
			"bin":{"a":"./a.js","b":"./b.js"},
			"dist-tags":{"latest":"1.0.0"},
			"versions":{"1.0.0":{"dependencies":{"dep-a":"^1"},"peerDependencies":{"peer-b":"*"}}}
		}`)
	}))
	t.Cleanup(srv.Close)
	doc, err := NewClient(srv.URL).GetDoc(context.Background(), "bar")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Repository != "https://bar.example/repo" || doc.License != "ISC" {
		t.Errorf("string forms: repo=%q lic=%q", doc.Repository, doc.License)
	}
	if doc.Bin["a"] != "./a.js" || doc.Bin["b"] != "./b.js" {
		t.Errorf("bin map = %v", doc.Bin)
	}
	if doc.Dependencies["dep-a"] != "^1" || doc.PeerDependencies["peer-b"] != "*" {
		t.Errorf("per-version overlay: deps=%v peer=%v", doc.Dependencies, doc.PeerDependencies)
	}
}
