package generated

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// Skip returns a copy of a that reports no diagnostic in a generated file.
//
// constable skips generated files because a diagnostic there is not
// actionable: the next run of the generator overwrites the file. An analyzer
// that constable embeds from another module does not know this policy, so
// Skip applies it.
func Skip(a *analysis.Analyzer) *analysis.Analyzer {
	skipped := *a
	run := a.Run
	skipped.Run = func(pass *analysis.Pass) (any, error) {
		generated := generatedFiles(pass, pass.Files)
		if len(generated) == 0 {
			return run(pass)
		}

		filtered := *pass
		report := pass.Report
		filtered.Report = func(diag analysis.Diagnostic) {
			if generated[pass.Fset.Position(diag.Pos).Filename] {
				return
			}
			report(diag)
		}
		return run(&filtered)
	}
	return &skipped
}

// generatedFiles returns the names of the generated files in files.
func generatedFiles(pass *analysis.Pass, files []*ast.File) map[string]bool {
	var names map[string]bool
	for _, file := range files {
		if !Skipped(file) {
			continue
		}
		if names == nil {
			names = make(map[string]bool)
		}
		names[pass.Fset.Position(file.Pos()).Filename] = true
	}
	return names
}
