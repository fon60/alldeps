package npmcmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const simpleFixture = `{
  "name": "lib",
  "dependencies": {
    "alpha": { "version": "1.0.0", "overridden": false },
    "beta": { "version": "2.3.4" }
  }
}`

const missingDepFixture = `{
  "name": "lib",
  "problems": ["missing: ghost@^1.0.0, required by broken-pkg@1.0.0"],
  "dependencies": {
    "broken-pkg": {
      "version": "1.0.0",
      "overridden": false,
      "dependencies": {
        "ghost": {
          "required": "^1.0.0",
          "missing": true,
          "problems": ["missing: ghost@^1.0.0, required by broken-pkg@1.0.0"]
        }
      }
    },
    "good-pkg": { "version": "2.0.0", "overridden": false },
    "npm": { "version": "11.19.1", "overridden": false,
      "dependencies": { "semver": { "version": "7.8.5" } } }
  },
  "error": { "code": "ELSPROBLEMS", "summary": "missing: ghost@^1.0.0, required by broken-pkg@1.0.0", "detail": "" }
}`

const invalidNodeFixture = `{
  "name": "lib",
  "dependencies": {
    "conflicted": {
      "version": "3.0.0",
      "invalid": true,
      "problems": ["invalid: conflicting peer dependency"]
    },
    "nested-bad": {
      "version": "1.0.0",
      "dependencies": {
        "deep": { "version": "0.1.0", "invalid": true }
      }
    }
  }
}`

func TestParseLSimple(t *testing.T) {
	pkgs, err := ParseLS([]byte(simpleFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages, want 2", len(pkgs))
	}
	if pkgs["alpha"].Version != "1.0.0" || pkgs["alpha"].Broken || pkgs["alpha"].Automatic {
		t.Fatalf("alpha = %+v, want 1.0.0 healthy direct", pkgs["alpha"])
	}
	if pkgs["beta"].Version != "2.3.4" || pkgs["beta"].Broken || pkgs["beta"].Automatic {
		t.Fatalf("beta = %+v, want 2.3.4 healthy direct", pkgs["beta"])
	}
}

func TestParseLSMissingDependency(t *testing.T) {
	pkgs, err := ParseLS([]byte(missingDepFixture))
	if err != nil {
		t.Fatal(err)
	}
	if !pkgs["broken-pkg"].Broken {
		t.Fatal("broken-pkg should be broken (missing dep in subtree)")
	}
	if pkgs["good-pkg"].Broken {
		t.Fatal("good-pkg should not be broken")
	}
	if pkgs["npm"].Broken {
		t.Fatal("npm should not be broken")
	}
	// The nested semver under npm is now listed as automatic; the missing
	// ghost (no version) yields no row.
	if len(pkgs) != 4 {
		t.Fatalf("got %d packages, want 4: %+v", len(pkgs), pkgs)
	}
	if p := pkgs["semver"]; !p.Automatic || p.Version != "7.8.5" || p.Broken {
		t.Fatalf("semver = %+v, want automatic 7.8.5 healthy", p)
	}
	if _, ok := pkgs["ghost"]; ok {
		t.Fatal("missing ghost must not be listed (it has no installed version)")
	}
}

// nestedTreeFixture exercises the full-tree walk: top-level names are direct
// even when they also appear nested, nested-only names are automatic, and the
// broken flag follows each name's own subtree.
const nestedTreeFixture = `{
  "name": "lib",
  "dependencies": {
    "root-a": {
      "version": "1.0.0",
      "dependencies": {
        "shared": { "version": "0.5.0" },
        "only-nested": {
          "version": "2.0.0",
          "dependencies": { "deep-missing": { "required": "^1.0.0", "missing": true } }
        }
      }
    },
    "root-b": {
      "version": "3.0.0",
      "dependencies": { "shared": { "version": "0.6.0" } }
    },
    "shared": { "version": "0.7.0" }
  }
}`

func TestParseLFullTreeUniqueNames(t *testing.T) {
	pkgs, err := ParseLS([]byte(nestedTreeFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 4 {
		t.Fatalf("got %d packages, want 4 (one per unique name): %+v", len(pkgs), pkgs)
	}
	// root-a's subtree reaches the missing deep-missing, so it stays broken
	// (the existing per-subtree Broken semantics are preserved).
	if p := pkgs["root-a"]; p.Automatic || p.Version != "1.0.0" || !p.Broken {
		t.Fatalf("root-a = %+v, want direct 1.0.0 broken", p)
	}
	if p := pkgs["root-b"]; p.Automatic || p.Version != "3.0.0" {
		t.Fatalf("root-b = %+v, want direct 3.0.0", p)
	}
	// shared is a top-level name: direct, at its own version, even though it
	// also appears nested under root-a and root-b.
	if p := pkgs["shared"]; p.Automatic || p.Version != "0.7.0" {
		t.Fatalf("shared = %+v, want direct 0.7.0 (top-level wins)", p)
	}
	// only-nested is reachable solely as a dependency and carries its own
	// missing-dep breakage.
	if p := pkgs["only-nested"]; !p.Automatic || p.Version != "2.0.0" || !p.Broken {
		t.Fatalf("only-nested = %+v, want automatic 2.0.0 broken", p)
	}
}

// platformSkipFixture mirrors real npm output for packages that bundle
// per-platform binaries as optional dependencies (e.g. opencode-ai): the
// non-matching platforms carry "invalid" as a reason string, and npm ls
// still exits 1 with ELSPROBLEMS.
const platformSkipFixture = `{
  "name": "lib",
  "problems": ["invalid: opencode-darwin-arm64@ /p/lib/node_modules/opencode-ai/node_modules/opencode-darwin-arm64"],
  "dependencies": {
    "opencode-ai": {
      "version": "1.18.29",
      "dependencies": {
        "opencode-linux-x64": { "version": "1.18.29" },
        "opencode-darwin-arm64": {
          "invalid": "\"1.18.29\" from node_modules/opencode-ai",
          "problems": ["invalid: opencode-darwin-arm64@ /p/lib/node_modules/opencode-ai/node_modules/opencode-darwin-arm64"]
        }
      }
    }
  },
  "error": { "code": "ELSPROBLEMS", "summary": "invalid: opencode-darwin-arm64@ /p/lib/node_modules/opencode-ai/node_modules/opencode-darwin-arm64", "detail": "" }
}`

func TestParseLSPlatformSkippedOptional(t *testing.T) {
	pkgs, err := ParseLS([]byte(platformSkipFixture))
	if err != nil {
		t.Fatal(err)
	}
	if pkgs["opencode-ai"].Broken {
		t.Fatal("platform-skipped optional deps must not mark the package broken")
	}
	if pkgs["opencode-ai"].Version != "1.18.29" {
		t.Fatalf("version = %q", pkgs["opencode-ai"].Version)
	}
}

func TestParseLSInvalidNodes(t *testing.T) {
	pkgs, err := ParseLS([]byte(invalidNodeFixture))
	if err != nil {
		t.Fatal(err)
	}
	if !pkgs["conflicted"].Broken {
		t.Fatal("conflicted should be broken (invalid flag)")
	}
	if !pkgs["nested-bad"].Broken {
		t.Fatal("nested-bad should be broken (invalid node in subtree)")
	}
}

func TestParseLEmpty(t *testing.T) {
	pkgs, err := ParseLS([]byte(`{"name":"lib","dependencies":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("got %d packages, want 0", len(pkgs))
	}
}

func TestParseLInvalidJSON(t *testing.T) {
	if _, err := ParseLS([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLocalDocAndReadme(t *testing.T) {
	dir := t.TempDir()
	pkgdir := filepath.Join(dir, "lib", "node_modules", "foo")
	if err := os.MkdirAll(pkgdir, 0o755); err != nil {
		t.Fatal(err)
	}
	pj := `{"name":"foo","version":"1.2.3","description":"local desc","license":"MIT","bin":{"foo":"cli.js"},"dependencies":{"bar":"^1.0.0"},"peerDependencies":{"baz":"*"}}`
	if err := os.WriteFile(filepath.Join(pkgdir, "package.json"), []byte(pj), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgdir, "README.md"), []byte("# foo\nlocal readme"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := LocalDoc(dir, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Description != "local desc" || doc.License != "MIT" || doc.Bin["foo"] != "cli.js" {
		t.Fatalf("doc = %+v", doc)
	}
	if len(doc.Versions) != 0 || doc.Readme != "" {
		t.Fatal("local doc must not carry versions/readme")
	}

	text, ok := Readme(dir, "foo")
	if !ok || !strings.Contains(text, "local readme") {
		t.Fatalf("readme = %q ok=%v", text, ok)
	}

	if _, err := LocalDoc(dir, "missing"); err == nil {
		t.Fatal("want error for missing package")
	}
	if _, ok := Readme(dir, "foo-noreadme"); ok {
		t.Fatal("want no readme for absent file")
	}
}

func TestNPMCommandStandardLayout(t *testing.T) {
	dir := t.TempDir()
	node := filepath.Join(dir, "bin", "node")
	npmCLI := filepath.Join(dir, "lib", "node_modules", "npm", "bin", "npm-cli.js")
	for _, p := range []string{node, npmCLI} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	argv, err := NPMCommand(context.Background(), dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) != 2 || argv[0] != node || argv[1] != npmCLI {
		t.Fatalf("argv = %v", argv)
	}
}

func TestNPMCommandFallbackToPathNPM(t *testing.T) {
	prefix := t.TempDir()
	bin := t.TempDir()
	npmPath := filepath.Join(bin, "npm")
	writeNPM := func(answer string) {
		script := "#!/bin/sh\nif [ \"$1\" = \"config\" ]; then echo " + answer + "; exit 0; fi\n"
		if err := os.WriteFile(npmPath, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeNPM("/srv/other")
	argv, err := NPMCommand(context.Background(), prefix, false)
	if err != nil {
		t.Fatalf("project mode should accept any PATH npm: %v", err)
	}
	if len(argv) != 1 || argv[0] != npmPath {
		t.Fatalf("argv = %v", argv)
	}

	if _, err := NPMCommand(context.Background(), prefix, true); err == nil {
		t.Fatal("strict mode must reject a PATH npm serving a different prefix")
	}

	writeNPM(prefix)
	if _, err := NPMCommand(context.Background(), prefix, true); err != nil {
		t.Fatalf("strict mode must accept a matching PATH npm: %v", err)
	}
}

func TestNPMCommandNoLayoutNoPathNPM(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := NPMCommand(context.Background(), t.TempDir(), false); err == nil {
		t.Fatal("want error when prefix lacks the standard layout and PATH has no npm")
	}
}
