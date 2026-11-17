package nilaway_test

import (
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	upstream "go.uber.org/nilaway"
	"go.uber.org/nilaway/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/nnutter/constable/internal/analysis/nilaway"
	"github.com/nnutter/constable/internal/generated"
)

// Compare at the analyzer boundary against unmodified upstream analysis with
// the same generated-file policy. Inferred contracts and facts cross package
// boundaries in the fixture, so dropping useful SSA can change diagnostics.
func TestScopedSSAMatchesUpstream(t *testing.T) {
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.LoadAllSyntax,
		Dir:  analysistest.TestData(),
		Env:  append(os.Environ(), "GO111MODULE=off", "GOPATH="+analysistest.TestData()),
	}, "scope/...")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	require.Zero(t, packages.PrintErrors(pkgs))

	cases := []struct {
		name    string
		include string
		exclude string
	}{
		{name: "all"},
		{name: "module", include: "scope"},
		{name: "client_only", include: "scope/client"},
		{name: "library_only", include: "scope/lib"},
		{name: "exclude_library", include: "scope", exclude: "scope/lib"},
		{name: "excluded", include: "another/module"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setIncludePkgs(t, tc.include)
			exclude := config.Analyzer.Flags.Lookup(config.ExcludePkgsFlag)
			require.NotNil(t, exclude)
			previous := exclude.Value.String()
			require.NoError(t, exclude.Value.Set(tc.exclude))
			t.Cleanup(func() { require.NoError(t, exclude.Value.Set(previous)) })
			want := analyzerDiagnostics(t, generated.Skip(upstream.Analyzer), pkgs)
			got := analyzerDiagnostics(t, nilaway.Analyzer, pkgs)
			require.Equal(t, want, got)
			if tc.name == "module" {
				require.NotEmpty(t, got, "the fixture must exercise reported nil flows")
			}
			if tc.name == "excluded" {
				require.Empty(t, got)
			}
		})
	}
}

func analyzerDiagnostics(t *testing.T, a *analysis.Analyzer, pkgs []*packages.Package) []string {
	t.Helper()
	graph, err := checker.Analyze([]*analysis.Analyzer{a}, pkgs, nil)
	require.NoError(t, err)
	var diagnostics []string
	for act := range graph.All() {
		require.NoError(t, act.Err)
		if act.IsRoot {
			for _, d := range act.Diagnostics {
				diagnostics = append(diagnostics, fmt.Sprintf("%s: %s", act.Package.Fset.Position(d.Pos), d.Message))
			}
		}
	}
	slices.Sort(diagnostics)
	return diagnostics
}
