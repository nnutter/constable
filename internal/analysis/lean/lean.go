package lean

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/generated"
	"github.com/nnutter/constable/internal/report"
)

var Analyzer = &analysis.Analyzer{
	Name: "lean",
	Doc:  "reports unused unexported declarations",
	Run:  run,
}

func succeeded(ok bool) bool {
	return ok
}

func isNilRecv(recv *ast.FieldList) bool {
	return recv == nil
}

func isBlankIdent(ident *ast.Ident) bool {
	return ident.Name == "_"
}

func isExported(name string) bool {
	pos := len(name)
	if pos == 0 {
		return false
	}
	return identIsExported(name)
}

func identIsExported(name string) bool {
	first := name[0]
	if first < 'A' {
		return false
	}
	return first <= 'Z'
}

type kind int

const (
	kindFunction kind = iota
	kindType
	kindVariable
	kindConstant
)

type symbol struct {
	obj  types.Object
	name string
	kind kind
	pos  token.Pos
	decl ast.Node
	file *ast.File
}

func run(pass *analysis.Pass) (any, error) {
	symbols := collectSymbols(pass)
	used := countUses(pass)
	reporter := newReporter(pass)
	for _, sym := range symbols {
		checkSymbol(pass, reporter, sym, used)
	}

	return nil, nil
}

func collectSymbols(pass *analysis.Pass) []symbol {
	var symbols []symbol
	for _, file := range pass.Files {
		if generated.Skipped(file) {
			continue
		}
		for _, decl := range file.Decls {
			symbols = append(symbols, symbolsForDecl(pass, file, decl)...)
		}
	}
	return symbols
}

func symbolsForDecl(pass *analysis.Pass, file *ast.File, decl ast.Decl) []symbol {
	if funcDecl, ok := decl.(*ast.FuncDecl); ok {
		return funcSymbol(pass, file, funcDecl)
	}
	if gen, ok := decl.(*ast.GenDecl); ok {
		return genSymbols(pass, file, gen)
	}
	return nil
}

func funcSymbol(pass *analysis.Pass, file *ast.File, funcDecl *ast.FuncDecl) []symbol {
	if !isNilRecv(funcDecl.Recv) {
		return nil
	}
	if isExported(funcDecl.Name.Name) {
		return nil
	}
	if isEntrypoint(funcDecl.Name.Name) {
		return nil
	}
	obj, ok := pass.TypesInfo.Defs[funcDecl.Name].(*types.Func)
	if !succeeded(ok) {
		return nil
	}
	if obj == nil {
		return nil
	}
	return []symbol{{
		obj:  obj,
		name: funcDecl.Name.Name,
		kind: kindFunction,
		pos:  funcDecl.Name.Pos(),
		decl: funcDecl,
		file: file,
	}}
}

func isEntrypoint(name string) bool {
	switch name {
	case "init", "main":
		return true
	}
	return false
}

func genSymbols(pass *analysis.Pass, file *ast.File, gen *ast.GenDecl) []symbol {
	switch gen.Tok {
	case token.TYPE:
		return typeSymbols(pass, file, gen)
	case token.VAR:
		return valueSymbols(pass, file, gen, kindVariable)
	case token.CONST:
		return constSymbols(pass, file, gen)
	}
	return nil
}

func constSymbols(pass *analysis.Pass, file *ast.File, gen *ast.GenDecl) []symbol {
	if isIotaSensitive(gen) {
		return nil
	}
	return valueSymbols(pass, file, gen, kindConstant)
}

func isIotaSensitive(gen *ast.GenDecl) bool {
	return slices.ContainsFunc(gen.Specs, isIotaSpec)
}

func isIotaSpec(spec ast.Spec) bool {
	valueSpec, ok := spec.(*ast.ValueSpec)
	if !ok {
		return false
	}
	if len(valueSpec.Values) == 0 {
		return true
	}
	return containsIota(valueSpec)
}

func containsIota(spec *ast.ValueSpec) bool {
	return slices.ContainsFunc(spec.Values, exprHasIota)
}

func exprHasIota(value ast.Expr) bool {
	found := false
	ast.Inspect(value, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok {
			if ident.Name == "iota" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func typeSymbols(pass *analysis.Pass, file *ast.File, gen *ast.GenDecl) []symbol {
	var symbols []symbol
	for _, spec := range gen.Specs {
		symbols = append(symbols, typeSymbol(pass, file, spec)...)
	}
	return symbols
}

func typeSymbol(pass *analysis.Pass, file *ast.File, spec ast.Spec) []symbol {
	typeSpec, ok := spec.(*ast.TypeSpec)
	if !ok {
		return nil
	}
	return typeSpecSymbols(pass, file, typeSpec)
}

func typeSpecSymbols(pass *analysis.Pass, file *ast.File, spec *ast.TypeSpec) []symbol {
	if isExported(spec.Name.Name) {
		return nil
	}
	if isBlankIdent(spec.Name) {
		return nil
	}
	obj, ok := pass.TypesInfo.Defs[spec.Name].(*types.TypeName)
	if !succeeded(ok) {
		return nil
	}
	if obj == nil {
		return nil
	}
	return []symbol{{
		obj:  obj,
		name: spec.Name.Name,
		kind: kindType,
		pos:  spec.Name.Pos(),
		decl: spec,
		file: file,
	}}
}

func valueSymbols(pass *analysis.Pass, file *ast.File, gen *ast.GenDecl, valueKind kind) []symbol {
	var symbols []symbol
	for _, spec := range gen.Specs {
		symbols = append(symbols, valueSpecSymbols(pass, file, spec, valueKind)...)
	}
	return symbols
}

func valueSpecSymbols(pass *analysis.Pass, file *ast.File, spec ast.Spec, valueKind kind) []symbol {
	valueSpec, ok := spec.(*ast.ValueSpec)
	if !ok {
		return nil
	}
	var symbols []symbol
	for _, name := range valueSpec.Names {
		symbols = append(symbols, valueNameSymbol(pass, file, valueSpec, name, valueKind)...)
	}
	return symbols
}

func valueNameSymbol(pass *analysis.Pass, file *ast.File, spec *ast.ValueSpec, name *ast.Ident, valueKind kind) []symbol {
	if isExported(name.Name) || isBlankIdent(name) {
		return nil
	}
	obj := pass.TypesInfo.Defs[name]
	if obj == nil {
		return nil
	}
	if !isValueKind(obj, valueKind) {
		return nil
	}
	return []symbol{{
		obj:  obj,
		name: name.Name,
		kind: valueKind,
		pos:  name.Pos(),
		decl: spec,
		file: file,
	}}
}

func isValueKind(obj types.Object, valueKind kind) bool {
	switch valueKind {
	case kindVariable:
		_, ok := obj.(*types.Var)
		return ok
	case kindConstant:
		_, ok := obj.(*types.Const)
		return ok
	}
	return false
}

func countUses(pass *analysis.Pass) map[types.Object]int {
	counts := make(map[types.Object]int)
	for _, obj := range pass.TypesInfo.Uses {
		if obj == nil {
			continue
		}
		counts[obj]++
	}
	return counts
}

func checkSymbol(pass *analysis.Pass, reporter *reporter, sym symbol, used map[types.Object]int) {
	if used[sym.obj] != 0 {
		return
	}
	reporter.report(pass, sym)
}

func messageFor(sym symbol) string {
	switch sym.kind {
	case kindFunction:
		return report.UnusedFunction(sym.name)
	case kindType:
		return report.UnusedType(sym.name)
	case kindVariable:
		return report.UnusedVariable(sym.name)
	case kindConstant:
		return report.UnusedConstant(sym.name)
	}
	return "unused symbol " + sym.name
}
