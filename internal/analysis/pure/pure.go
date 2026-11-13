package pure

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/directive"
	"github.com/nnutter/constable/internal/directive/enum"
	"github.com/nnutter/constable/internal/generated"
	"github.com/nnutter/constable/internal/report"
)

var Analyzer = &analysis.Analyzer{
	Name: enum.PureName,
	Doc:  "reports observable effects in //constable:pure functions",
	Run:  run,
}

type origin struct {
	name       string
	typ        types.Type
	isReceiver bool
}

type functionState struct {
	pass    *analysis.Pass
	origins map[types.Object]origin
	globals map[types.Object]string
}

func succeeded(ok bool) bool {
	return ok
}

func isNilBody(body *ast.BlockStmt) bool {
	return body == nil
}

func isChanged(changed bool) bool {
	return changed
}

func isDeleteName(name string) bool {
	return name == "delete"
}

func hasNoArgs(call *ast.CallExpr) bool {
	return len(call.Args) == 0
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if generated.Skipped(file) {
			continue
		}

		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !succeeded(ok) || !hasPureDirective(funcDecl) || isNilBody(funcDecl.Body) {
				continue
			}

			state := newFunctionState(pass, funcDecl)
			state.collectAliases(funcDecl.Body)
			state.reportEffects(funcDecl.Body)
		}
	}

	return nil, nil
}

func hasPureDirective(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Doc == nil {
		return false
	}
	for _, comment := range funcDecl.Doc.List {
		if directive.HasPure(comment.Text) {
			return true
		}
	}
	return false
}

func newFunctionState(pass *analysis.Pass, funcDecl *ast.FuncDecl) *functionState {
	state := &functionState{
		pass:    pass,
		origins: make(map[types.Object]origin),
		globals: make(map[types.Object]string),
	}

	if funcDecl.Recv != nil {
		state.addFields(funcDecl.Recv, true)
	}
	if funcDecl.Type.Params != nil {
		state.addFields(funcDecl.Type.Params, false)
	}

	return state
}

func isAddrOp(expr *ast.UnaryExpr) bool {
	return expr.Op == token.AND
}

func isPackageVar(obj types.Object) bool {
	if obj == nil {
		return false
	}
	if obj.Pkg() == nil {
		return false
	}
	if _, ok := obj.(*types.Var); !ok {
		return false
	}
	return obj.Parent() == obj.Pkg().Scope()
}

func qualifiedName(obj types.Object) string {
	if obj == nil {
		return ""
	}
	if builtin, ok := obj.(*types.Builtin); ok {
		return builtin.Name()
	}
	if obj.Pkg() == nil {
		return obj.Name()
	}
	return obj.Pkg().Name() + "." + obj.Name()
}

func isImpureObject(obj types.Object) bool {
	if obj == nil {
		return false
	}
	if builtin, ok := obj.(*types.Builtin); ok {
		return isPrintBuiltin(builtin.Name())
	}
	if obj.Pkg() == nil {
		return false
	}
	return isImpurePackage(obj.Pkg().Path(), obj.Name())
}

func isPrintBuiltin(name string) bool {
	switch name {
	case "print", "println":
		return true
	}
	return false
}

func isImpurePackage(pkgPath, name string) bool {
	switch pkgPath {
	case "log":
		return true
	case "math/rand", "math/rand/v2":
		return true
	case "fmt":
		return isFmtOutput(name)
	case "time":
		return isClockFunc(name)
	case "os":
		return isOsEffect(name)
	}
	return false
}

func isFmtOutput(name string) bool {
	switch name {
	case "Print", "Printf", "Println", "Fprint", "Fprintf", "Fprintln":
		return true
	}
	return false
}

func isClockFunc(name string) bool {
	switch name {
	case "Now", "Since", "Until", "Sleep", "After", "AfterFunc", "Tick", "NewTicker", "NewTimer":
		return true
	}
	return false
}

func isOsEffect(name string) bool {
	switch name {
	case "IsNotExist", "IsExist", "IsPermission", "IsTimeout", "Name":
		return false
	}
	return true
}

func isAmbientVar(obj types.Object) bool {
	if obj == nil {
		return false
	}
	if _, ok := obj.(*types.Var); !ok {
		return false
	}
	if obj.Pkg() == nil {
		return false
	}
	return isAmbientPackageVar(obj.Pkg().Path(), obj.Name())
}

func isAmbientPackageVar(pkgPath, name string) bool {
	switch pkgPath {
	case "os":
		return isOsAmbient(name)
	case "time":
		return name == "Local"
	}
	return false
}

func isOsAmbient(name string) bool {
	switch name {
	case "Args", "Stdout", "Stderr", "Stdin":
		return true
	}
	return false
}

func ambientName(obj types.Object) string {
	if obj == nil {
		return ""
	}
	if obj.Pkg() == nil {
		return obj.Name()
	}
	return obj.Pkg().Name() + "." + obj.Name()
}

func collectLeftSelectors(body *ast.BlockStmt) map[*ast.SelectorExpr]bool {
	lefts := make(map[*ast.SelectorExpr]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if sel, ok := unparen(lhs).(*ast.SelectorExpr); ok {
					lefts[sel] = true
				}
			}
		case *ast.IncDecStmt:
			if sel, ok := unparen(node.X).(*ast.SelectorExpr); ok {
				lefts[sel] = true
			}
		}

		return true
	})
	return lefts
}

func isReferenceLike(typ types.Type) bool {
	typ = dereference(typ)
	switch typ.(type) {
	case *types.Pointer, *types.Slice, *types.Map:
		return true
	}

	return false
}

func isIndexReferenceLike(typ types.Type) bool {
	typ = dereference(typ)
	switch typ.(type) {
	case *types.Slice, *types.Map:
		return true
	}

	return false
}

func dereference(typ types.Type) types.Type {
	for {
		named, ok := typ.(*types.Named)
		if !ok {
			return typ
		}

		typ = named.Underlying()
	}
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}

		expr = paren.X
	}
}

func (state *functionState) addFields(fields *ast.FieldList, isReceiver bool) {
	for _, field := range fields.List {
		for _, name := range field.Names {
			obj := state.pass.TypesInfo.Defs[name]
			if obj == nil {
				continue
			}

			state.origins[obj] = origin{
				name:       name.Name,
				typ:        obj.Type(),
				isReceiver: isReceiver,
			}
		}
	}
}

func (state *functionState) callObject(call *ast.CallExpr) types.Object {
	return state.funObject(call.Fun)
}

func (state *functionState) callObjectIndex(expr *ast.IndexExpr) types.Object {
	inner := unparen(expr.X)
	switch inner := inner.(type) {
	case *ast.SelectorExpr:
		return state.selectorObject(inner)
	case *ast.Ident:
		return state.pass.TypesInfo.Uses[inner]
	}

	return nil
}

func (state *functionState) collectAliases(body *ast.BlockStmt) {
	changed := true
	for changed {
		changed = false
		ast.Inspect(body, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.AssignStmt:
				changed = state.collectAssignAliases(node) || isChanged(changed)
			}

			return true
		})
	}
}

func (state *functionState) collectAssignAliases(stmt *ast.AssignStmt) bool {
	changed := false
	for i, lhs := range stmt.Lhs {
		if i >= len(stmt.Rhs) {
			continue
		}
		changed = state.collectOneAlias(lhs, stmt.Rhs[i]) || isChanged(changed)
	}

	return changed
}

func (state *functionState) collectOneAlias(lhs ast.Expr, rhs ast.Expr) bool {
	ident, ok := lhs.(*ast.Ident)
	if !ok {
		return false
	}

	obj := state.objectOf(ident)
	if obj == nil {
		return false
	}
	if state.trackParamAlias(obj, rhs) {
		return true
	}
	return state.trackGlobalAlias(obj, rhs)
}

func (state *functionState) exprRefersToCallerVisibleMemory(expr ast.Expr) (origin, bool) {
	expr = unparen(expr)
	switch expr := expr.(type) {
	case *ast.Ident:
		orig, ok := state.originOf(expr)
		if !ok {
			return orig, false
		}

		return orig, isReferenceLike(state.typeOf(expr))
	case *ast.SelectorExpr:
		if orig, ok := state.exprRefersToCallerVisibleMemory(expr.X); ok {
			return orig, true
		}

		orig, ok := state.rootOrigin(expr.X)
		if !ok {
			return orig, false
		}

		return orig, isReferenceLike(state.typeOf(expr))
	case *ast.IndexExpr:
		orig, ok := state.rootOrigin(expr.X)
		if !ok {
			return orig, false
		}

		return orig, isIndexReferenceLike(state.typeOf(expr.X))
	case *ast.StarExpr:
		return state.exprRefersToCallerVisibleMemory(expr.X)
	}

	return origin{}, false
}

func (state *functionState) funObject(expr ast.Expr) types.Object {
	fun := unparen(expr)
	switch fun := fun.(type) {
	case *ast.SelectorExpr:
		return state.selectorObject(fun)
	case *ast.IndexExpr:
		return state.callObjectIndex(fun)
	case *ast.IndexListExpr:
		return state.funObject(fun.X)
	case *ast.Ident:
		return state.pass.TypesInfo.Uses[fun]
	}

	return nil
}

func (state *functionState) globalOf(expr ast.Expr) (string, bool) {
	expr = unparen(expr)
	switch expr := expr.(type) {
	case *ast.Ident:
		obj := state.objectOf(expr)
		if obj == nil {
			return "", false
		}
		if isPackageVar(obj) {
			return obj.Name(), true
		}
		name, ok := state.globals[obj]
		return name, ok
	case *ast.SelectorExpr:
		obj := state.pass.TypesInfo.Uses[expr.Sel]
		if obj == nil {
			return "", false
		}
		if isPackageVar(obj) {
			return obj.Name(), true
		}
	case *ast.UnaryExpr:
		if isAddrOp(expr) {
			return state.globalOf(expr.X)
		}
	}

	return "", false
}

func (state *functionState) message(orig origin, isDelete bool) string {
	if isDelete {
		return report.PureDeletesFromMapParameter(orig.name)
	}
	if orig.isReceiver {
		return report.PureMutatesReceiver(orig.name)
	}
	if _, ok := dereference(orig.typ).(*types.Pointer); ok {
		return report.PureMutatesPointerParameter(orig.name)
	}

	return report.PureMutatesParameter(orig.name)
}

func (state *functionState) objectOf(ident *ast.Ident) types.Object {
	if obj := state.pass.TypesInfo.Defs[ident]; obj != nil {
		return obj
	}

	return state.pass.TypesInfo.Uses[ident]
}

func (state *functionState) originOf(expr ast.Expr) (origin, bool) {
	expr = unparen(expr)
	switch expr := expr.(type) {
	case *ast.Ident:
		obj := state.objectOf(expr)
		orig, ok := state.origins[obj]
		return orig, ok
	}

	return origin{}, false
}

func (state *functionState) reportAssigns(stmt *ast.AssignStmt) {
	for _, lhs := range stmt.Lhs {
		state.reportMutationTarget(lhs)
	}
}

func (state *functionState) reportCall(call *ast.CallExpr) {
	state.reportDelete(call)
	state.reportImpureCall(call)
}

func (state *functionState) reportDelete(call *ast.CallExpr) {
	ident, ok := call.Fun.(*ast.Ident)
	if !succeeded(ok) || !isDeleteName(ident.Name) || hasNoArgs(call) {
		return
	}

	if orig, ok := state.exprRefersToCallerVisibleMemory(call.Args[0]); ok {
		state.pass.Report(analysis.Diagnostic{
			Pos:     call.Pos(),
			Message: state.message(orig, true),
		})
	}
	if name, ok := state.globalOf(call.Args[0]); ok {
		state.pass.Report(analysis.Diagnostic{
			Pos:     call.Pos(),
			Message: report.PureWritesGlobal(name),
		})
	}
}

func (state *functionState) reportEffects(body *ast.BlockStmt) {
	lefts := collectLeftSelectors(body)
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.AssignStmt:
			state.reportAssigns(node)
		case *ast.IncDecStmt:
			state.reportMutationTarget(node.X)
		case *ast.CallExpr:
			state.reportCall(node)
		case *ast.GoStmt:
			state.pass.Report(analysis.Diagnostic{
				Pos:     node.Pos(),
				Message: report.PureSpawnsGoroutine(),
			})
		case *ast.SelectorExpr:
			state.reportSelectorRead(node, lefts)
		}

		return true
	})
}

func (state *functionState) reportImpureCall(call *ast.CallExpr) {
	obj := state.callObject(call)
	if obj == nil {
		return
	}
	if !isImpureObject(obj) {
		return
	}
	state.pass.Report(analysis.Diagnostic{
		Pos:     call.Pos(),
		Message: report.PureCalls(qualifiedName(obj)),
	})
}

func (state *functionState) reportMutationTarget(expr ast.Expr) {
	if orig, ok := state.visibleMutationOrigin(expr); ok {
		state.pass.Report(analysis.Diagnostic{
			Pos:     expr.Pos(),
			Message: state.message(orig, false),
		})
	}
	if name, ok := state.visibleGlobalName(expr); ok {
		state.pass.Report(analysis.Diagnostic{
			Pos:     expr.Pos(),
			Message: report.PureWritesGlobal(name),
		})
	}
}

func (state *functionState) reportSelectorRead(sel *ast.SelectorExpr, lefts map[*ast.SelectorExpr]bool) {
	if lefts[sel] {
		return
	}
	obj := state.pass.TypesInfo.Uses[sel.Sel]
	if obj == nil {
		return
	}
	if isAmbientVar(obj) {
		state.pass.Report(analysis.Diagnostic{
			Pos:     sel.Pos(),
			Message: report.PureReadsVar(ambientName(obj)),
		})
	}
}

func (state *functionState) rootGlobalName(expr ast.Expr) (string, bool) {
	expr = unparen(expr)
	switch expr := expr.(type) {
	case *ast.Ident:
		obj := state.objectOf(expr)
		if obj == nil {
			return "", false
		}
		if isPackageVar(obj) {
			return obj.Name(), true
		}
		name, ok := state.globals[obj]
		return name, ok
	case *ast.SelectorExpr:
		obj := state.pass.TypesInfo.Uses[expr.Sel]
		if obj == nil {
			return state.rootGlobalName(expr.X)
		}
		if isPackageVar(obj) {
			return obj.Name(), true
		}
		return state.rootGlobalName(expr.X)
	case *ast.StarExpr:
		return state.rootGlobalName(expr.X)
	case *ast.IndexExpr:
		return state.rootGlobalName(expr.X)
	}

	return "", false
}

func (state *functionState) rootOrigin(expr ast.Expr) (origin, bool) {
	expr = unparen(expr)
	switch expr := expr.(type) {
	case *ast.Ident:
		return state.originOf(expr)
	case *ast.StarExpr:
		return state.rootOrigin(expr.X)
	case *ast.SelectorExpr:
		return state.rootOrigin(expr.X)
	case *ast.IndexExpr:
		return state.rootOrigin(expr.X)
	}

	return origin{}, false
}

func (state *functionState) selectorObject(sel *ast.SelectorExpr) types.Object {
	if selObj := state.pass.TypesInfo.Selections[sel]; selObj != nil {
		return selObj.Obj()
	}
	return state.pass.TypesInfo.Uses[sel.Sel]
}

func (state *functionState) trackGlobalAlias(obj types.Object, rhs ast.Expr) bool {
	if _, exists := state.globals[obj]; exists {
		return false
	}

	name, ok := state.globalOf(rhs)
	if !succeeded(ok) {
		return false
	}

	state.globals[obj] = name
	return true
}

func (state *functionState) trackParamAlias(obj types.Object, rhs ast.Expr) bool {
	if _, exists := state.origins[obj]; exists {
		return false
	}

	orig, ok := state.originOf(rhs)
	if !succeeded(ok) {
		return false
	}

	state.origins[obj] = orig
	return true
}

func (state *functionState) typeOf(expr ast.Expr) types.Type {
	return state.pass.TypesInfo.TypeOf(expr)
}

func (state *functionState) visibleGlobalName(expr ast.Expr) (string, bool) {
	expr = unparen(expr)
	switch expr := expr.(type) {
	case *ast.Ident:
		obj := state.objectOf(expr)
		if obj == nil {
			return "", false
		}
		if isPackageVar(obj) {
			return obj.Name(), true
		}
		return "", false
	case *ast.SelectorExpr:
		obj := state.pass.TypesInfo.Uses[expr.Sel]
		if obj == nil {
			return state.rootGlobalName(expr.X)
		}
		if isPackageVar(obj) {
			return obj.Name(), true
		}
		return state.rootGlobalName(expr.X)
	case *ast.StarExpr:
		return state.rootGlobalName(expr.X)
	case *ast.IndexExpr:
		return state.rootGlobalName(expr.X)
	}

	return "", false
}

func (state *functionState) visibleMutationOrigin(expr ast.Expr) (origin, bool) {
	expr = unparen(expr)
	switch expr := expr.(type) {
	case *ast.Ident:
		return origin{}, false
	case *ast.StarExpr:
		return state.exprRefersToCallerVisibleMemory(expr.X)
	case *ast.SelectorExpr:
		return state.exprRefersToCallerVisibleMemory(expr.X)
	case *ast.IndexExpr:
		return state.exprRefersToCallerVisibleMemory(expr.X)
	}

	return origin{}, false
}
