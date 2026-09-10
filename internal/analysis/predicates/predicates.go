package predicates

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/report"
)

var Analyzer = &analysis.Analyzer{
	Name: "predicates",
	Doc:  "reports compound conditionals that use variables instead of predicate functions",
	Run:  run,
}

func succeeded(ok bool) bool {
	return ok
}

func isLogicalExpr(expr *ast.BinaryExpr) bool {
	if expr == nil {
		return false
	}
	switch expr.Op {
	case token.LAND, token.LOR:
		return true
	default:
		return false
	}
}

func isNot(unary *ast.UnaryExpr) bool {
	if unary == nil {
		return false
	}
	return unary.Op == token.NOT
}

func isBoolName(ident *ast.Ident) bool {
	if ident == nil {
		return false
	}
	switch ident.Name {
	case "true", "false", "nil":
		return true
	default:
		return false
	}
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			expr, ok := node.(*ast.BinaryExpr)
			if !succeeded(ok) || !isLogicalExpr(expr) {
				return true
			}
			// The first operand is always evaluated, so statement coverage
			// can observe short-circuiting through the remaining calls.
			checkOperand(pass, expr.Y)
			return true
		})
	}

	return nil, nil
}

func checkOperand(pass *analysis.Pass, operand ast.Expr) {
	stripped := stripParens(operand)
	if isCompound(stripped) || isPredicateOperand(stripped) || isBoolLiteral(stripped) {
		return
	}
	if unary, ok := stripped.(*ast.UnaryExpr); succeeded(ok) && isNot(unary) {
		inner := stripParens(unary.X)
		if isBoolLiteral(inner) || isPredicateOperand(inner) {
			return
		}
	}

	pass.Report(analysis.Diagnostic{
		Pos:     operand.Pos(),
		End:     operand.End(),
		Message: report.UsePredicateInstead(exprString(pass.Fset, stripped)),
	})
}

func stripParens(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

func isCompound(expr ast.Expr) bool {
	binary, ok := expr.(*ast.BinaryExpr)
	return succeeded(ok) && isLogicalExpr(binary)
}

func isPredicateOperand(expr ast.Expr) bool {
	if _, ok := expr.(*ast.CallExpr); ok {
		return true
	}
	if unary, ok := expr.(*ast.UnaryExpr); succeeded(ok) && isNot(unary) {
		return isPredicateOperand(stripParens(unary.X))
	}
	return false
}

func isBoolLiteral(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return succeeded(ok) && isBoolName(ident)
}

func exprString(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, expr)
	return buf.String()
}
