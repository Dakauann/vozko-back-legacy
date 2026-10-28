package siteinspect

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const inspectImport = "vozko/usecases/siteinspect"

var publishPackages = []string{
	"../sitepublish",
	"../../domain/sitepublish",
	"../../delivery/http/sitepublishhttp",
}

var advisoryKeys = []string{inspectionKeyPrefix, outlineKeyPrefix, outlineLockPrefix}

func publishSources(t *testing.T) map[string]*ast.File {
	t.Helper()
	files := map[string]*ast.File{}
	set := token.NewFileSet()
	for _, dir := range publishPackages {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			parsed, err := parser.ParseFile(set, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			files[path] = parsed
		}
	}
	if len(files) == 0 {
		t.Fatal("no publish sources were found")
	}
	return files
}

func TestThePublishPathNeverReadsABrowserFedInspection(t *testing.T) {
	for path, file := range publishSources(t) {
		for _, spec := range file.Imports {
			if imported, _ := strconv.Unquote(spec.Path.Value); imported == inspectImport {
				t.Errorf("%s imports %s; publish gates must come from its own server side render", path, inspectImport)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, _ := strconv.Unquote(literal.Value)
			for _, key := range advisoryKeys {
				if strings.Contains(value, strings.TrimSuffix(key, ":")) {
					t.Errorf("%s names the advisory key %q", path, key)
				}
			}
			return true
		})
	}
}

func TestAdvisoryKeysNeverShareAPrefixWithAnotherStore(t *testing.T) {
	key, ok := inspectionKey("build-1", []string{"/"}, view().Settings.Settings)
	if !ok || !strings.HasPrefix(key, "site:inspect:advisory:build-1:") {
		t.Fatalf("key %q", key)
	}
	if !strings.HasPrefix(outlineKeyPrefix, "site:outline:") || !strings.HasPrefix(outlineLockPrefix, outlineKeyPrefix) {
		t.Fatalf("outline keys %q %q", outlineKeyPrefix, outlineLockPrefix)
	}
}
