package generated_test

import (
	"go/ast"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/generated"
)

// reportFuncs reports every function declaration, so a test can observe which
// files the analyzer reaches.
var reportFuncs = &analysis.Analyzer{
	Name: "reportfuncs",
	Doc:  "reports every function declaration",
	Run: func(pass *analysis.Pass) (any, error) {
		for _, file := range pass.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				pass.Reportf(fn.Name.Pos(), "func %s", fn.Name.Name)
			}
		}
		return nil, nil
	},
}

func TestSkip(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), generated.Skip(reportFuncs), "g")
}

func TestSkipWithInclude(t *testing.T) {
	generated.Include(true)
	t.Cleanup(func() { generated.Include(false) })

	analysistest.Run(t, analysistest.TestData(), generated.Skip(reportFuncs), "h")
}
