package driver_test

import (
	"testing"

	"github.com/nnutter/gittest"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/analysis/nilaway"
	"github.com/nnutter/constable/internal/analysis/nonmutating"
	"github.com/nnutter/constable/internal/driver"
)

// BenchmarkRun includes package loading on every iteration, as separate CLI
// invocations do. The directive package provides a small real workload with
// standard-library and test dependencies. Go's build cache remains warm.
func BenchmarkRun(b *testing.B) {
	gittest.WithSetenvEnviron(b)
	b.Chdir("../..")
	cases := []struct {
		name      string
		analyzers []*analysis.Analyzer
		fix       bool
	}{
		{name: "syntax", analyzers: []*analysis.Analyzer{nonmutating.Analyzer}},
		{name: "facts", analyzers: []*analysis.Analyzer{nilaway.Analyzer}},
		{name: "facts_fix_noop", analyzers: []*analysis.Analyzer{nilaway.Analyzer}, fix: true},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			opts := driver.Options{
				Context:      -1,
				IncludeTests: true,
				Fix:          tc.fix,
				AfterLoad:    nilaway.UseMainModules,
			}
			b.ReportAllocs()
			for b.Loop() {
				require.Equal(b, 0, driver.Run([]string{"./internal/directive"}, tc.analyzers, opts))
			}
		})
	}
}
