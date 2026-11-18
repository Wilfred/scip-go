package index_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scip-code/scip-go/internal/config"
	"github.com/scip-code/scip-go/internal/index"
	"github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

func writePackageLoadFixture(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func indexPackageLoadFixture(root string, patterns []string) (int, error) {
	documents := 0
	err := index.Index(func(msg proto.Message) error {
		if _, ok := msg.(*scip.Document); ok {
			documents++
		}
		return nil
	}, config.IndexOpts{ModuleRoot: root, ModuleVersion: "test", SkipTests: true, SkipImplementations: true, PackagePatterns: patterns})
	return documents, err
}

func TestIndexPackageLoadingErrors(t *testing.T) {
	t.Setenv("GOWORK", "off")
	for _, tc := range []struct {
		name       string
		files      map[string]string
		patterns   []string
		diagnostic string
	}{
		{name: "nested module", files: map[string]string{
			"nested/go.mod":    "module example/nested\ngo 1.26\n",
			"nested/source.go": "package nested\nfunc F() {}\n",
		}, diagnostic: "directory prefix"},
		{name: "invalid pattern", files: map[string]string{
			"go.mod":    "module example\ngo 1.26\n",
			"source.go": "package example\nfunc F() {}\n",
		}, patterns: []string{"./missing"}, diagnostic: "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writePackageLoadFixture(t, root, tc.files)
			documents, err := indexPackageLoadFixture(root, tc.patterns)
			if err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("expected diagnostic %q, got %v", tc.diagnostic, err)
			}
			if documents != 0 {
				t.Fatalf("got %d documents, want none", documents)
			}
		})
	}
}

func TestIndexValidEmptyPackages(t *testing.T) {
	t.Setenv("GOWORK", "off")
	for _, tc := range []struct {
		name     string
		source   string
		patterns []string
	}{
		{name: "empty module"},
		{name: "unmatched pattern", source: "package example\nfunc F() {}\n", patterns: []string{"./empty/..."}},
		{name: "excluded by build tags", source: "//go:build ignore\n\npackage example\nfunc F() {}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{"go.mod": "module example\ngo 1.26\n"}
			if tc.source != "" {
				files["source.go"] = tc.source
			}
			if len(tc.patterns) > 0 {
				files["empty/README.md"] = "No Go packages in this directory.\n"
			}
			writePackageLoadFixture(t, root, files)
			documents, err := indexPackageLoadFixture(root, tc.patterns)
			if err != nil || documents != 0 {
				t.Fatalf("empty index: documents=%d, error=%v", documents, err)
			}
		})
	}
}

func TestIndexWorkspacePackagePatterns(t *testing.T) {
	root := t.TempDir()
	writePackageLoadFixture(t, root, map[string]string{
		"go.work":     "go 1.26\nuse (\n ./a\n ./b\n)\n",
		"a/go.mod":    "module example/a\ngo 1.26\n",
		"a/source.go": "package a\nfunc F() {}\n",
		"b/go.mod":    "module example/b\ngo 1.26\n",
		"b/source.go": "package b\nfunc F() {}\n",
	})
	t.Setenv("GOWORK", filepath.Join(root, "go.work"))
	if documents, err := indexPackageLoadFixture(root, nil); err == nil || documents != 0 {
		t.Fatalf("default workspace pattern: documents=%d, error=%v", documents, err)
	}
	if documents, err := indexPackageLoadFixture(root, []string{"./a/...", "./b/..."}); err != nil || documents != 2 {
		t.Fatalf("explicit workspace patterns: documents=%d, error=%v", documents, err)
	}
}

func TestIndexPartialPackages(t *testing.T) {
	t.Setenv("GOWORK", "off")
	root := t.TempDir()
	writePackageLoadFixture(t, root, map[string]string{
		"go.mod":    "module example\ngo 1.26\n",
		"source.go": "package example\nvar X = missing\n",
	})
	documents, err := indexPackageLoadFixture(root, []string{".", "./missing"})
	if err != nil || documents != 1 {
		t.Fatalf("partial index: documents=%d, error=%v", documents, err)
	}
}
