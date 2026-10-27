package container

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEveryStageWriterHoldsTheAnnouncingRepository(t *testing.T) {
	boot := containerFuncs(t)["New"]
	if boot.decl == nil {
		t.Fatal("New not found")
	}
	var order []string
	for _, stmt := range boot.decl.Body.List {
		call, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		if expr, ok := call.X.(*ast.CallExpr); ok {
			order = append(order, types.ExprString(expr.Fun))
		}
	}
	repos, announce := -1, -1
	for i, name := range order {
		switch name {
		case "c.initRepositories":
			repos = i
		case "c.announceStageEntries":
			announce = i
		}
	}
	if repos < 0 || announce != repos+1 {
		t.Fatalf("boot order %v: the stage repository must be wrapped right after it is built, before any use case captures it", order)
	}
}

func TestNoProductionCodeBuildsASecondStageRepository(t *testing.T) {
	const stagePackage = "vozko/infra/repositories/stage"
	allowed := map[string]bool{
		filepath.Join("infra", "container", "repositories.go"): true,
	}
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "node_modules" || strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, filepath.Join("infra", "repositories", "stage")) || allowed[rel] {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			if imported, _ := strconv.Unquote(spec.Path.Value); imported == stagePackage {
				t.Errorf("%s imports the stage repository; stage writes outside the container's announcing repository never fire the stage trigger", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
