package unmodified

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"

	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/generated"
	"github.com/nnutter/constable/internal/report"
)

var Analyzer = &analysis.Analyzer{
	Name: "unmodified",
	Doc:  "reports variables that are never modified and can be constants",
	Run:  run,
}

func succeeded(ok bool) bool {
	return ok
}

func isVarToken(tok token.Token) bool {
	return tok == token.VAR
}

func isAssignToken(tok token.Token) bool {
	return tok == token.ASSIGN
}

func isAddressToken(tok token.Token) bool {
	return tok == token.AND
}

func isBlankName(name string) bool {
	return name == "_"
}

func isNilObject(obj types.Object) bool {
	return obj == nil
}

func isNilVar(obj *types.Var) bool {
	return obj == nil
}

type candidate struct {
	obj  *types.Var
	name *ast.Ident
	init ast.Expr
	spec *specInfo
}

type specInfo struct {
	spec     *ast.ValueSpec
	gen      *ast.GenDecl
	pkgLevel bool
	names    []*candidate
	complete bool
	fixable  bool
}

type declInfo struct {
	gen   *ast.GenDecl
	specs []*specInfo
}

func run(pass *analysis.Pass) (any, error) {
	mutated := mutatedObjects(pass)
	specs, decls := collectSpecs(pass)
	suppressed := suppressedDecls(pass)
	markComplete(pass, specs, mutated)
	markFixable(decls)
	for _, spec := range specs {
		reportSpec(pass, spec, mutated, suppressed)
	}
	return nil, nil
}

func reportSpec(pass *analysis.Pass, spec *specInfo, mutated map[types.Object]bool, suppressed map[*ast.GenDecl]bool) {
	if suppressed[spec.gen] {
		return
	}
	for _, cand := range spec.names {
		if !isEligible(pass, cand, mutated) {
			continue
		}
		diag := analysis.Diagnostic{
			Pos:     cand.name.Pos(),
			Message: report.UnmodifiedVariable(cand.name.Name),
		}
		if spec.fixable {
			diag.SuggestedFixes = []analysis.SuggestedFix{*suggestedFix(spec.gen, cand.name.Name)}
		}
		pass.Report(diag)
	}
}

func suggestedFix(gen *ast.GenDecl, name string) *analysis.SuggestedFix {
	return &analysis.SuggestedFix{
		Message: "Replace var with const for " + name,
		TextEdits: []analysis.TextEdit{{
			Pos:     gen.TokPos,
			End:     gen.TokPos + token.Pos(len("var")),
			NewText: []byte("const"),
		}},
	}
}

func markComplete(pass *analysis.Pass, specs []*specInfo, mutated map[types.Object]bool) {
	for _, spec := range specs {
		spec.complete = specComplete(pass, spec, mutated)
	}
}

func markFixable(decls map[*ast.GenDecl]*declInfo) {
	for _, decl := range decls {
		fixable := declFixable(decl)
		for _, spec := range decl.specs {
			spec.fixable = fixable
		}
	}
}

func declFixable(decl *declInfo) bool {
	if decl.gen.Lparen.IsValid() {
		return allComplete(decl.specs)
	}
	if len(decl.specs) != 1 {
		return false
	}
	return decl.specs[0].complete
}

func allComplete(specs []*specInfo) bool {
	if len(specs) == 0 {
		return false
	}
	for _, spec := range specs {
		if !spec.complete {
			return false
		}
	}
	return true
}

func specComplete(pass *analysis.Pass, spec *specInfo, mutated map[types.Object]bool) bool {
	if len(spec.spec.Values) == 0 {
		return false
	}
	if len(spec.spec.Names) != len(spec.spec.Values) {
		return false
	}
	if len(spec.names) != len(spec.spec.Names) {
		return false
	}
	for _, cand := range spec.names {
		if !isEligible(pass, cand, mutated) {
			return false
		}
	}
	return true
}

func isEligible(pass *analysis.Pass, cand *candidate, mutated map[types.Object]bool) bool {
	if isNilVar(cand.obj) {
		return false
	}
	if isBlankName(cand.name.Name) {
		return false
	}
	if cand.spec.pkgLevel && ast.IsExported(cand.name.Name) {
		return false
	}
	if mutated[cand.obj] {
		return false
	}
	if !isConstantExpr(pass, cand.init) {
		return false
	}
	return hasConstType(cand.obj)
}

func isConstantExpr(pass *analysis.Pass, expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	return pass.TypesInfo.Types[expr].Value != nil
}

func hasConstType(obj *types.Var) bool {
	basic, ok := obj.Type().Underlying().(*types.Basic)
	if !succeeded(ok) {
		return false
	}
	return isConstKind(basic.Kind())
}

func isConstKind(kind types.BasicKind) bool {
	switch kind {
	case types.Bool, types.String,
		types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr,
		types.Float32, types.Float64, types.Complex64, types.Complex128,
		types.UntypedBool, types.UntypedString, types.UntypedInt, types.UntypedRune,
		types.UntypedFloat, types.UntypedComplex:
		return true
	default:
		return false
	}
}

func collectSpecs(pass *analysis.Pass) ([]*specInfo, map[*ast.GenDecl]*declInfo) {
	var specs []*specInfo
	decls := make(map[*ast.GenDecl]*declInfo)
	for _, file := range pass.Files {
		if generated.Skipped(file) {
			continue
		}
		fileSpecs, fileDecls := collectFileSpecs(pass, file)
		specs = append(specs, fileSpecs...)
		maps.Copy(decls, fileDecls)
	}
	return specs, decls
}

func collectFileSpecs(pass *analysis.Pass, file *ast.File) ([]*specInfo, map[*ast.GenDecl]*declInfo) {
	topLevel := topLevelDecls(file)
	var specs []*specInfo
	decls := make(map[*ast.GenDecl]*declInfo)
	ast.Inspect(file, func(node ast.Node) bool {
		gen, ok := node.(*ast.GenDecl)
		if !succeeded(ok) || !isVarToken(gen.Tok) {
			return true
		}
		decl := &declInfo{gen: gen}
		decls[gen] = decl
		for _, rawSpec := range gen.Specs {
			spec := specFor(pass, gen, rawSpec, topLevel[gen])
			if spec == nil {
				continue
			}
			decl.specs = append(decl.specs, spec)
			specs = append(specs, spec)
		}
		return true
	})
	return specs, decls
}

func topLevelDecls(file *ast.File) map[*ast.GenDecl]bool {
	topLevel := make(map[*ast.GenDecl]bool)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if succeeded(ok) {
			topLevel[gen] = true
		}
	}
	return topLevel
}

func specFor(pass *analysis.Pass, gen *ast.GenDecl, rawSpec ast.Spec, pkgLevel bool) *specInfo {
	valueSpec, ok := rawSpec.(*ast.ValueSpec)
	if !succeeded(ok) {
		return nil
	}
	spec := &specInfo{spec: valueSpec, gen: gen, pkgLevel: pkgLevel}
	if len(valueSpec.Values) != len(valueSpec.Names) {
		return spec
	}
	for i, name := range valueSpec.Names {
		spec.names = append(spec.names, candidateFor(pass, spec, name, valueSpec.Values[i]))
	}
	return spec
}

func candidateFor(pass *analysis.Pass, spec *specInfo, name *ast.Ident, init ast.Expr) *candidate {
	obj, _ := pass.TypesInfo.Defs[name].(*types.Var)
	return &candidate{obj: obj, name: name, init: init, spec: spec}
}

func mutatedObjects(pass *analysis.Pass) map[types.Object]bool {
	mutated := make(map[types.Object]bool)
	for _, file := range pass.Files {
		if generated.Skipped(file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			markNode(pass, node, mutated)
			return true
		})
	}
	return mutated
}

func markNode(pass *analysis.Pass, node ast.Node, mutated map[types.Object]bool) {
	switch node := node.(type) {
	case *ast.AssignStmt:
		for _, lhs := range node.Lhs {
			markExpr(pass, lhs, mutated)
		}
	case *ast.RangeStmt:
		markRange(pass, node, mutated)
	case *ast.IncDecStmt:
		markExpr(pass, node.X, mutated)
	case *ast.UnaryExpr:
		markAddress(pass, node, mutated)
	case *ast.SelectorExpr:
		markMethod(pass, node, mutated)
	}
}

func markRange(pass *analysis.Pass, stmt *ast.RangeStmt, mutated map[types.Object]bool) {
	if !isAssignToken(stmt.Tok) {
		return
	}
	markExpr(pass, stmt.Key, mutated)
	if stmt.Value != nil {
		markExpr(pass, stmt.Value, mutated)
	}
}

func markAddress(pass *analysis.Pass, expr *ast.UnaryExpr, mutated map[types.Object]bool) {
	if isAddressToken(expr.Op) {
		markExpr(pass, expr.X, mutated)
	}
}

func markMethod(pass *analysis.Pass, expr *ast.SelectorExpr, mutated map[types.Object]bool) {
	if isPointerMethod(pass, expr) {
		markExpr(pass, expr.X, mutated)
	}
}

func markExpr(pass *analysis.Pass, expr ast.Expr, mutated map[types.Object]bool) {
	ident := rootIdent(expr)
	if ident == nil {
		return
	}
	obj := objectFor(pass, ident)
	if isNilObject(obj) {
		return
	}
	mutated[obj] = true
}

func rootIdent(expr ast.Expr) *ast.Ident {
	for expr != nil {
		switch e := expr.(type) {
		case *ast.Ident:
			return e
		case *ast.ParenExpr:
			expr = e.X
		case *ast.SelectorExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.StarExpr:
			expr = e.X
		default:
			return nil
		}
	}
	return nil
}

func objectFor(pass *analysis.Pass, ident *ast.Ident) types.Object {
	if obj := pass.TypesInfo.Uses[ident]; !isNilObject(obj) {
		return obj
	}
	return pass.TypesInfo.Defs[ident]
}

func isPointerMethod(pass *analysis.Pass, expr *ast.SelectorExpr) bool {
	selection, ok := pass.TypesInfo.Selections[expr]
	if !succeeded(ok) || !isMethodValue(selection) {
		return false
	}
	fn, ok := selection.Obj().(*types.Func)
	if !succeeded(ok) || isNilFunc(fn) {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !succeeded(ok) || isNilSignature(sig) {
		return false
	}
	if sig.Recv() == nil {
		return false
	}
	_, isPtr := sig.Recv().Type().(*types.Pointer)
	return isPtr
}

func isMethodValue(selection *types.Selection) bool {
	if selection == nil {
		return false
	}
	return selection.Kind() == types.MethodVal
}

func isNilFunc(fn *types.Func) bool {
	return fn == nil
}

func isNilSignature(sig *types.Signature) bool {
	return sig == nil
}
