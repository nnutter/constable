package testify

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/report"
)

func succeeded(ok bool) bool {
	return ok
}

func isNilSelection(selection *types.Selection) bool {
	return selection == nil
}

func isNilObject(obj types.Object) bool {
	return obj == nil
}

func isNilPackage(pkg *types.Package) bool {
	return pkg == nil
}

func isTestingPath(path string) bool {
	return path == "testing"
}

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
	if !succeeded(ok) || isNilSelection(selection) {
		return
	}
	obj := selection.Obj()
	if isNilObject(obj) || isNilPackage(obj.Pkg()) || !isTestingPath(obj.Pkg().Path()) {
		return
	}

	pass.Report(analysis.Diagnostic{
		Pos:     sel.Sel.Pos(),
		Message: report.UseTestifyInstead(sel.Sel.Name, testifyPackage),
	})
}
