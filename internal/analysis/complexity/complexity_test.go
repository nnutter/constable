package complexity_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/complexity"
)

func TestAnalyzer(t *testing.T) {
	require.NoError(t, complexity.Analyzer.Flags.Set("limit", "10"))
	analysistest.Run(t, analysistest.TestData(), complexity.Analyzer, "c")
}

func TestAnalyzerCustomLimit(t *testing.T) {
	require.NoError(t, complexity.Analyzer.Flags.Set("limit", "2"))
	// Restore the default so flag state does not leak into other tests
	// when they run shuffled.
	t.Cleanup(func() {
		require.NoError(t, complexity.Analyzer.Flags.Set("limit", "10"))
	})
	analysistest.Run(t, analysistest.TestData(), complexity.Analyzer, "d")
}
