package nilaway_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	upstream "go.uber.org/nilaway"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"

	"github.com/nnutter/constable/internal/analysis/nilaway"
	"github.com/nnutter/constable/internal/generated"
)

func TestSuppressionFactsMatchUpstream(t *testing.T) {
	setIncludePkgs(t, "suppression")
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.LoadAllSyntax,
		Dir:  analysistest.TestData(),
		Env:  append(os.Environ(), "GO111MODULE=off", "GOPATH="+analysistest.TestData()),
	}, "suppression/...")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	require.Zero(t, packages.PrintErrors(pkgs))

	want := analyzerDiagnostics(t, generated.Skip(upstream.Analyzer), pkgs)
	require.NotEmpty(t, want, "the unsuppressed nil flow must still be reported")
	got := analyzerDiagnostics(t, nilaway.Analyzer, pkgs)
	require.Equal(t, want, got)
}
