package nilaway

import (
	"slices"

	"go.uber.org/nilaway/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

// scopeSSA copies NilAway's requirement graph so that its SSA prerequisite
// observes the same scope as the function-contract analyzer that consumes it.
// The checker runs prerequisites before their consumers, so the consumer's
// own scope check cannot prevent building SSA for excluded dependencies.
// Copy only the changed prerequisite and its consumers. Other prerequisites
// retain their identities because analyzers outside NilAway may share their
// fact types. Upstream analyzers remain unchanged for other users.
func scopeSSA(root *analysis.Analyzer) *analysis.Analyzer {
	copies := make(map[*analysis.Analyzer]*analysis.Analyzer)
	var clone func(*analysis.Analyzer) *analysis.Analyzer
	clone = func(a *analysis.Analyzer) *analysis.Analyzer {
		if copy := copies[a]; copy != nil {
			return copy
		}
		copy := *a
		copies[a] = &copy
		requires, run := a.Requires, a.Run
		changed := a == buildssa.Analyzer
		if changed {
			requires = append(slices.Clone(requires), config.Analyzer)
			run = runScopedSSA
		}
		copy.Requires = make([]*analysis.Analyzer, len(requires))
		for i, req := range requires {
			copy.Requires[i] = clone(req)
			if copy.Requires[i] != req {
				changed = true
			}
		}
		if !changed {
			copies[a] = a
			return a
		}
		copy.Run = originalPass(a, requires, copies, run)
		return &copy
	}
	return clone(root)
}

// originalPass restores the analyzer identities that upstream Run functions
// use to look up prerequisite results. Copy the pass rather than changing the
// checker's pass, including when another adapter has installed an overlay.
func originalPass(a *analysis.Analyzer, requires []*analysis.Analyzer, copies map[*analysis.Analyzer]*analysis.Analyzer, run func(*analysis.Pass) (any, error)) func(*analysis.Pass) (any, error) {
	return func(pass *analysis.Pass) (any, error) {
		original := *pass
		original.Analyzer = a
		original.ResultOf = make(map[*analysis.Analyzer]any, len(requires))
		for _, req := range requires {
			original.ResultOf[req] = pass.ResultOf[copies[req]]
		}
		return run(&original)
	}
}

func runScopedSSA(pass *analysis.Pass) (any, error) {
	conf := pass.ResultOf[config.Analyzer].(*config.Config)
	if !conf.IsPkgInScope(pass.Pkg) {
		// The function-contract analyzer returns before reading SSA for these
		// packages. Return the declared result type without constructing SSA.
		return new(buildssa.SSA), nil
	}
	return buildssa.Analyzer.Run(pass)
}
