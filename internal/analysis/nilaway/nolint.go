package nilaway

import (
	"go/ast"
	"strings"

	"go.uber.org/nilaway/diagnostic"
	"golang.org/x/tools/go/analysis"
)

// skipEmptyNoLintFiles avoids constructing comment maps for files without
// suppression comments. Still run the upstream analyzer for every package:
// it imports and exports suppression facts used by downstream diagnostics,
// even for packages outside NilAway's configured analysis scope.
func skipEmptyNoLintFiles(root *analysis.Analyzer) *analysis.Analyzer {
	filtered := *diagnostic.NoLintAnalyzer
	filtered.Run = func(pass *analysis.Pass) (any, error) {
		copy := *pass
		copy.Files = nil
		for _, file := range pass.Files {
			if mayHaveNoLint(file) {
				copy.Files = append(copy.Files, file)
			}
		}
		return diagnostic.NoLintAnalyzer.Run(&copy)
	}
	return replacePrerequisite(root, diagnostic.NoLintAnalyzer, &filtered)
}

func mayHaveNoLint(file *ast.File) bool {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			// Deliberately accept more than the upstream parser. It remains
			// responsible for recognizing valid directives and their ranges.
			if strings.Contains(comment.Text, "nolint") {
				return true
			}
		}
	}
	return false
}
