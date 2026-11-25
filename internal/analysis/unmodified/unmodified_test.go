package unmodified_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/unmodified"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), unmodified.Analyzer, "u")
}

func TestAnalyzerRespectsNoLint(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), unmodified.Analyzer, "n")
}

func TestAnalyzerSkipsGeneratedFiles(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), unmodified.Analyzer, "g")
}

func TestSuggestedFixes(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), unmodified.Analyzer, "fix")
}
