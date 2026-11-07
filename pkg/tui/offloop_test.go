package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A fetch runs off the loop, so it may name m only in the apply it returns,
// which Update runs on it.
func TestFetchesLeaveTheModelToTheirApply(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fset := token.NewFileSet()
	fetches := 0

	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)

		ast.Inspect(file, func(node ast.Node) bool {
			fetch, ok := node.(*ast.FuncLit)
			if !ok || !isFetch(fetch.Type) {
				return true
			}

			fetches++

			for _, use := range offLoopUses(fetch.Body) {
				assert.Failf(t, "fetch reads the model off the loop", "%s: m.%s", fset.Position(use.Pos()), use.Sel.Name)
			}

			return false
		})
	}

	assert.Greater(t, fetches, 10, "the fetches were found")
}

// isFetch is func() (func(), error).
func isFetch(fn *ast.FuncType) bool {
	if fn.Params.NumFields() != 0 || fn.Results.NumFields() != 2 {
		return false
	}

	apply, ok := fn.Results.List[0].Type.(*ast.FuncType)
	errType, isIdent := fn.Results.List[1].Type.(*ast.Ident)

	return ok && isApply(apply) && isIdent && errType.Name == "error"
}

// isApply is func(), what a fetch hands back for the loop to run.
func isApply(fn *ast.FuncType) bool {
	return fn.Params.NumFields() == 0 && fn.Results.NumFields() == 0
}

func offLoopUses(body *ast.BlockStmt) []*ast.SelectorExpr {
	var uses []*ast.SelectorExpr

	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.FuncLit:
			return !isApply(node.Type)
		case *ast.SelectorExpr:
			if ident, ok := node.X.(*ast.Ident); ok && ident.Name == "m" {
				uses = append(uses, node)
			}
		}

		return true
	})

	return uses
}
