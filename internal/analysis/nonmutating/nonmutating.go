package nonmutating

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"

	"github.com/nnutter/constable/internal/directive"
	"github.com/nnutter/constable/internal/directive/enum"
	"github.com/nnutter/constable/internal/report"
)

var Analyzer = &analysis.Analyzer{
	Name: enum.NonmutatingName,
	Doc:  "reports caller-visible argument mutations in //constable:nonmutating functions",
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
	},
	Run: run,
}

type origin struct {
	name       string
	typ        types.Type
	isReceiver bool
}

type functionState struct {
	pass    *analysis.Pass
	origins map[types.Object]origin
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || !hasNonmutatingDirective(funcDecl) || funcDecl.Body == nil {
				continue
			}

			state := newFunctionState(pass, funcDecl)
			state.collectAliases(funcDecl.Body)
			state.reportMutations(funcDecl.Body)
		}
	}

	return nil, nil
}

func hasNonmutatingDirective(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Doc == nil {
		return false
	}
	for _, comment := range funcDecl.Doc.List {
		if directive.HasNonmutating(comment.Text) {
			return true
		}
	}
	return false
}

func newFunctionState(pass *analysis.Pass, funcDecl *ast.FuncDecl) *functionState {
	state := &functionState{
		pass:    pass,
		origins: make(map[types.Object]origin),
	}

	if funcDecl.Recv != nil {
		state.addFields(funcDecl.Recv, true)
	}
	if funcDecl.Type.Params != nil {
		state.addFields(funcDecl.Type.Params, false)
	}

	return state
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

func (state *functionState) collectAliases(body *ast.BlockStmt) {
	changed := true
	for changed {
		changed = false
		ast.Inspect(body, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.AssignStmt:
				changed = state.collectAssignAliases(node) || changed
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

		ident, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}

		obj := state.objectOf(ident)
		origin, ok := state.originOf(stmt.Rhs[i])
		if obj == nil || !ok {
			continue
		}

		if _, exists := state.origins[obj]; !exists {
			state.origins[obj] = origin
			changed = true
		}
	}

	return changed
}

func (state *functionState) containerStorageVisible(expr ast.Expr) (origin, bool) {
	expr = unparen(expr)
	switch expr := expr.(type) {
	case *ast.Ident:
		origin, ok := state.originOf(expr)
		if !ok {
			return origin, false
		}

		return origin, isReferenceLike(state.typeOf(expr))
	default:
		panic("unexpected expr type: " + fmt.Sprintf("%T", expr))
	}
}

func (state *functionState) exprRefersToCallerVisibleMemory(expr ast.Expr) (origin, bool) {
	expr = unparen(expr)
	switch expr := expr.(type) {
	case *ast.Ident:
		origin, ok := state.originOf(expr)
		if !ok {
			return origin, false
		}

		return origin, isReferenceLike(state.typeOf(expr))
	case *ast.SelectorExpr:
		origin, ok := state.originOf(expr.X)
		if !ok {
			return origin, false
		}

		if _, ok := state.containerStorageVisible(expr.X); ok {
			return origin, true
		}

		return origin, isReferenceLike(state.typeOf(expr))
	case *ast.IndexExpr:
		origin, ok := state.originOf(expr.X)
		if !ok {
			return origin, false
		}

		return origin, isIndexReferenceLike(state.typeOf(expr.X))
	case *ast.StarExpr:
		return state.exprRefersToCallerVisibleMemory(expr.X)
	}

	return origin{}, false
}

func (state *functionState) message(origin origin, isDelete bool) string {
	if isDelete {
		return report.DeletesFromMapParameter(origin.name)
	}
	if origin.isReceiver {
		return report.MutatesReceiver(origin.name)
	}
	if _, ok := dereference(origin.typ).(*types.Pointer); ok {
		return report.MutatesPointerParameter(origin.name)
	}

	return report.MutatesParameter(origin.name)
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
		origin, ok := state.origins[obj]
		return origin, ok
	}

	return origin{}, false
}

func (state *functionState) reportDelete(call *ast.CallExpr) {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != "delete" || len(call.Args) == 0 {
		return
	}

	origin, ok := state.exprRefersToCallerVisibleMemory(call.Args[0])
	if !ok {
		return
	}

	state.pass.Report(analysis.Diagnostic{
		Pos:     call.Pos(),
		Message: state.message(origin, true),
	})
}

func (state *functionState) reportMutation(expr ast.Expr, isDelete bool) {
	origin, ok := state.visibleMutationOrigin(expr)
	if !ok {
		return
	}

	state.pass.Report(analysis.Diagnostic{
		Pos:     expr.Pos(),
		Message: state.message(origin, isDelete),
	})
}

func (state *functionState) reportMutations(body *ast.BlockStmt) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				state.reportMutation(lhs, false)
			}
		case *ast.IncDecStmt:
			state.reportMutation(node.X, false)
		case *ast.CallExpr:
			state.reportDelete(node)
		}

		return true
	})
}

func (state *functionState) typeOf(expr ast.Expr) types.Type {
	return state.pass.TypesInfo.TypeOf(expr)
}

func (state *functionState) visibleMutationOrigin(expr ast.Expr) (origin, bool) {
	expr = unparen(expr)
	switch expr := expr.(type) {
	case *ast.Ident:
		return origin{}, false
	case *ast.StarExpr:
		return state.exprRefersToCallerVisibleMemory(expr.X)
	case *ast.SelectorExpr:
		return state.containerStorageVisible(expr.X)
	case *ast.IndexExpr:
		return state.exprRefersToCallerVisibleMemory(expr.X)
	}

	return origin{}, false
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
