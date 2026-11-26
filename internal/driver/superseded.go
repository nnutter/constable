package driver

import (
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis/checker"
)

// supersededAnalyzers lists analyzers whose findings about a package
// are only sound when test files are part of the analysis. The plain
// package "p" is analyzed alongside its test variant "p [p.test]", and
// the plain pass cannot see test-only uses such as a package variable
// that is only assigned in a test.
var supersededAnalyzers = map[string]bool{"unmodified": true}

type diagnosticKey struct {
	pos     token.Position
	message string
}

// isSupersedable reports whether the action is a successful root run of an
// analyzer that supersededAnalyzers names.
func isSupersedable(act *checker.Action) bool {
	if !act.IsRoot {
		return false
	}
	if act.Err != nil {
		return false
	}
	return supersededAnalyzers[act.Analyzer.Name]
}

func diagnosticKeys(act *checker.Action) map[diagnosticKey]bool {
	keys := make(map[diagnosticKey]bool)
	for _, diag := range act.Diagnostics {
		keys[diagnosticKey{act.Package.Fset.Position(diag.Pos), diag.Message}] = true
	}
	return keys
}

// testVariantDiagnostics maps each analyzer and base package to the
// diagnostics that the test variant of the package reported.
func testVariantDiagnostics(graph *checker.Graph) map[string]map[diagnosticKey]bool {
	variants := make(map[string]map[diagnosticKey]bool)
	for act := range graph.All() {
		if !isSupersedable(act) {
			continue
		}
		base, _, ok := strings.Cut(act.Package.ID, " [")
		if !ok {
			continue
		}
		variants[act.Analyzer.Name+"\x00"+base] = diagnosticKeys(act)
	}
	return variants
}

// dropSupersededDiagnostics removes diagnostics that the plain package
// pass reported but its test variant, which sees strictly more code,
// did not.
func dropSupersededDiagnostics(graph *checker.Graph) {
	variants := testVariantDiagnostics(graph)
	for act := range graph.All() {
		if !isSupersedable(act) {
			continue
		}
		keys, ok := variants[act.Analyzer.Name+"\x00"+act.Package.ID]
		if !ok {
			continue
		}
		kept := act.Diagnostics[:0]
		for _, diag := range act.Diagnostics {
			if keys[diagnosticKey{act.Package.Fset.Position(diag.Pos), diag.Message}] {
				kept = append(kept, diag)
			}
		}
		act.Diagnostics = kept
	}
}
