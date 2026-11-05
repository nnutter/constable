package driver

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
)

// flagUsage returns the usage of a registered flag, or an empty string so that
// the test reports the missing flag instead of a panic.
func flagUsage(t *testing.T, fs *flag.FlagSet, name string) string {
	t.Helper()

	f := fs.Lookup(name)
	require.NotNil(t, f, "flag %s is not registered", name)
	// The nil check states that a flag can be missing, so the analyzer that
	// looks for nil dereferences sees the guard that require makes.
	if f == nil {
		return ""
	}
	return f.Usage
}

// flagValue returns the value of a registered flag, or an empty string so that
// the test reports the missing flag instead of a panic.
func flagValue(t *testing.T, fs *flag.FlagSet, name string) string {
	t.Helper()

	f := fs.Lookup(name)
	require.NotNil(t, f, "flag %s is not registered", name)
	// The nil check states that a flag can be missing, so the analyzer that
	// looks for nil dereferences sees the guard that require makes.
	if f == nil {
		return ""
	}
	return f.Value.String()
}

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
	assert.Equal(t, "true", flagValue(t, fs, "required.on"))
	assert.Equal(t, "example.com/m", flagValue(t, fs, "required.scope"))
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

	assert.Equal(t, "a flag that the driver already defines", flagUsage(t, fs, "analyzer.on"))
}
