package lean_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/lean"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), lean.Analyzer, "l")
}

func TestAnalyzerSkipsGeneratedFiles(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), lean.Analyzer, "g")
}

func TestSuggestedFixes(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), lean.Analyzer, "fix")
}
