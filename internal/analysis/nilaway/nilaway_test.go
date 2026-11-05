package nilaway_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/nilaway/config"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"

	"github.com/nnutter/constable/internal/analysis/nilaway"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), nilaway.Analyzer, "na")
}

// includePkgs returns the package list that the configuration analyzer read.
func includePkgs(t *testing.T) string {
	t.Helper()

	flag := config.Analyzer.Flags.Lookup(config.IncludePkgsFlag)
	require.NotNil(t, flag)
	return flag.Value.String()
}

// setIncludePkgs records the value that a test gives the flag, and clears it
// afterwards so that the flag does not leak into another test.
func setIncludePkgs(t *testing.T, value string) {
	t.Helper()

	flag := config.Analyzer.Flags.Lookup(config.IncludePkgsFlag)
	require.NotNil(t, flag)
	require.NoError(t, flag.Value.Set(value))
	t.Cleanup(func() {
		require.NoError(t, flag.Value.Set(""))
	})
}

// setIncludeAllPkgs sets the adapter flag, and restores it afterwards.
func setIncludeAllPkgs(t *testing.T, value string) {
	t.Helper()

	require.NoError(t, nilaway.Analyzer.Flags.Set("include-all-pkgs", value))
	t.Cleanup(func() {
		require.NoError(t, nilaway.Analyzer.Flags.Set("include-all-pkgs", "false"))
	})
}

func moduleRoot(path string, main bool) *packages.Package {
	return &packages.Package{PkgPath: path + "/pkg", Module: &packages.Module{Path: path, Main: main}}
}

func TestUseMainModules(t *testing.T) {
	setIncludePkgs(t, "")

	nilaway.UseMainModules([]*packages.Package{
		moduleRoot("example.com/m", true),
		moduleRoot("example.com/m", true),
		moduleRoot("example.com/dep", false),
		{PkgPath: "fmt", Module: nil},
		nil,
	})

	assert.Equal(t, "example.com/m", includePkgs(t))
}

func TestUseMainModulesCoversEveryMainModule(t *testing.T) {
	setIncludePkgs(t, "")

	nilaway.UseMainModules([]*packages.Package{
		moduleRoot("example.com/one", true),
		moduleRoot("example.com/two", true),
	})

	assert.Equal(t, "example.com/one,example.com/two", includePkgs(t))
}

func TestUseMainModulesKeepsChosenPackages(t *testing.T) {
	setIncludePkgs(t, "example.com/chosen")

	nilaway.UseMainModules([]*packages.Package{moduleRoot("example.com/m", true)})

	assert.Equal(t, "example.com/chosen", includePkgs(t))
}

func TestUseMainModulesWithAllPackages(t *testing.T) {
	setIncludePkgs(t, "")
	setIncludeAllPkgs(t, "true")

	nilaway.UseMainModules([]*packages.Package{moduleRoot("example.com/m", true)})

	assert.Empty(t, includePkgs(t))
}

func TestUseMainModulesWithoutModule(t *testing.T) {
	setIncludePkgs(t, "")

	nilaway.UseMainModules([]*packages.Package{{PkgPath: "fmt", Module: nil}})

	assert.Empty(t, includePkgs(t))
}
