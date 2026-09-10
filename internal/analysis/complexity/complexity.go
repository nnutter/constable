package complexity

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/report"
)

const defaultComplexityLimit = 10

var limit = defaultComplexityLimit

var Analyzer = &analysis.Analyzer{
	Name: "complexity",
	Doc:  "reports functions with cyclomatic complexity over the limit",
	Run:  run,
}

func init() {
	Analyzer.Flags.IntVar(&limit, "limit", defaultComplexityLimit, "maximum allowed cyclomatic complexity")
}

func isFuncDecl(ok bool) bool {
	return ok
}

func isNilBody(body *ast.BlockStmt) bool {
	return body == nil
}

func isLogicalAnd(op token.Token) bool {
	return op == token.LAND
}

func isLogicalOr(op token.Token) bool {
	return op == token.LOR
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !isFuncDecl(ok) || isNilBody(funcDecl.Body) {
				continue
			}

			complexity := measure(funcDecl.Body)
			if complexity > limit {
				pass.Report(analysis.Diagnostic{
					Pos:     funcDecl.Name.Pos(),
					Message: report.ComplexityExceedsLimit(funcDecl.Name.Name, complexity, limit),
				})
			}
		}
	}

	return nil, nil
}

// measure returns the cyclomatic complexity of a function body.
// The count starts at 1 and adds 1 for each if, for, range, case,
// comm, &&, and || node. Closures fold into the enclosing function
// and default clauses count the same as other clauses.
func measure(body *ast.BlockStmt) int {
	complexity := 1
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.IfStmt,
			*ast.ForStmt,
			*ast.RangeStmt,
			*ast.CaseClause,
			*ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			if isLogicalAnd(node.Op) || isLogicalOr(node.Op) {
				complexity++
			}
		}
		return true
	})
	return complexity
}
