package unmodified

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// suppressedDecls returns the var declarations that carry a
// //nolint:unmodified comment, keyed by declaration. The comment map
// attaches a comment to the largest node it belongs to, NilAway's rule,
// so a comment above a var block suppresses the whole block and one
// trailing a single var suppresses that declaration. A report on any
// name inside a suppressed declaration is dropped.
func suppressedDecls(pass *analysis.Pass) map[*ast.GenDecl]bool {
	suppressed := make(map[*ast.GenDecl]bool)
	for _, file := range pass.Files {
		collectSuppressed(pass, file, suppressed)
	}
	return suppressed
}

func collectSuppressed(pass *analysis.Pass, file *ast.File, suppressed map[*ast.GenDecl]bool) {
	parents := parentMap(file)
	commentMap := ast.NewCommentMap(pass.Fset, file, file.Comments)
	for node, groups := range commentMap {
		if !commentMapHasNoLint(groups) {
			continue
		}
		if gen := outerVarDecl(parents, node); gen != nil {
			suppressed[gen] = true
		}
	}
}

// parentMap maps every node in file to the node that directly contains it.
func parentMap(file *ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

// outerVarDecl returns the innermost var declaration enclosing node, or
// nil when node sits inside no var declaration. A DeclStmt owns the var
// declaration it wraps, so a statement maps to that declaration directly.
// The walk stops at the first declaration boundary: a FuncDecl, FuncLit,
// or non-var GenDecl can never be a var declaration itself.
func outerVarDecl(parents map[ast.Node]ast.Node, node ast.Node) *ast.GenDecl {
	for node != nil {
		switch node := node.(type) {
		case *ast.GenDecl:
			if node.Tok == token.VAR {
				return node
			}
			return nil
		case *ast.DeclStmt:
			if gen, ok := node.Decl.(*ast.GenDecl); ok {
				if gen.Tok == token.VAR {
					return gen
				}
			}
			return nil
		case *ast.FuncDecl, *ast.FuncLit:
			return nil
		}
		node = parents[node]
	}
	return nil
}

func commentMapHasNoLint(groups []*ast.CommentGroup) bool {
	for _, group := range groups {
		for _, comment := range group.List {
			if nolintUnmodified(comment.Text) {
				return true
			}
		}
	}
	return false
}

// nolintUnmodified reports whether a comment suppresses the unmodified
// analyzer. The parser mirrors NilAway's: a bare //nolint does not name
// this analyzer, and an explanation after // is ignored.
func nolintUnmodified(text string) bool {
	text = strings.TrimLeft(text, "/ ")
	if !strings.HasPrefix(text, "nolint") {
		return false
	}
	split := strings.Split(text, "//")
	text = strings.TrimSpace(split[0])
	parts := strings.Split(text, ":")
	if len(parts) == 1 {
		return false
	}
	for linter := range strings.SplitSeq(strings.TrimSpace(parts[1]), ",") {
		if strings.EqualFold(strings.TrimSpace(linter), "unmodified") {
			return true
		}
	}
	return false
}
