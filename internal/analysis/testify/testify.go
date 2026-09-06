package testify

import (
	"go/ast"

	"github.com/nnutter/constable/internal/report"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "testify",
	Doc:  "reports testing failure calls that should use testify assert or require",
	Run:  run,
}

var testifyPackages = map[string]string{
	"Error":   "assert",
	"Errorf":  "assert",
	"Fail":    "assert",
	"FailNow": "require",
	"Fatal":   "require",
	"Fatalf":  "require",
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			checkCall(pass, call)
			return true
		})
	}

	return nil, nil
}

func checkCall(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	testifyPackage, ok := testifyPackages[sel.Sel.Name]
	if !ok {
		return
	}

	selection, ok := pass.TypesInfo.Selections[sel]
	if !ok || selection == nil {
		return
	}
	obj := selection.Obj()
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "testing" {
		return
	}

	pass.Report(analysis.Diagnostic{
		Pos:     sel.Sel.Pos(),
		Message: report.UseTestifyInstead(sel.Sel.Name, testifyPackage),
	})
}
