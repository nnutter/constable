package driver

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
)

func newAnalyzer(name string, requires ...*analysis.Analyzer) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     name,
		Doc:      name,
		Run:      func(*analysis.Pass) (any, error) { return nil, nil },
		Requires: requires,
	}
}

func TestRegisterFlagsRegistersRequiredAnalyzerFlags(t *testing.T) {
	required := newAnalyzer("required")
	required.Flags.Bool("on", false, "turn the required analyzer on")
	required.Flags.String("scope", "", "limit the packages that the required analyzer reads")

	root := newAnalyzer("root", required)
	root.Flags.String("limit", "10", "limit of the root analyzer")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	registerFlags(fs, []*analysis.Analyzer{root})

	assert.NotNil(t, fs.Lookup("root.limit"))
	assert.NotNil(t, fs.Lookup("required.on"))
	assert.NotNil(t, fs.Lookup("required.scope"))
	assert.Nil(t, fs.Lookup("root.on"), "a flag of a required analyzer does not belong to the root")
	assert.Nil(t, fs.Lookup("required"), "a required analyzer has no enable flag")

	require.NoError(t, fs.Parse([]string{"-required.on", "-required.scope=example.com/m"}))
	assert.Equal(t, "true", fs.Lookup("required.on").Value.String())
	assert.Equal(t, "example.com/m", fs.Lookup("required.scope").Value.String())
}

func TestRegisterFlagsVisitsAnAnalyzerOnce(t *testing.T) {
	shared := newAnalyzer("shared")
	shared.Flags.Bool("on", false, "turn the shared analyzer on")

	first := newAnalyzer("first", shared)
	second := newAnalyzer("second", shared)

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	registerFlags(fs, []*analysis.Analyzer{first, second})

	assert.NotNil(t, fs.Lookup("shared.on"))
	assert.Nil(t, fs.Lookup("second.on"))
}

func TestRegisterFlagsKeepsAnExistingFlag(t *testing.T) {
	analyzer := newAnalyzer("analyzer")
	analyzer.Flags.Bool("on", false, "turn the analyzer on")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Bool("analyzer.on", false, "a flag that the driver already defines")
	registerFlags(fs, []*analysis.Analyzer{analyzer})

	assert.Equal(t, "a flag that the driver already defines", fs.Lookup("analyzer.on").Usage)
}
