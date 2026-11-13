package pure_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/pure"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), pure.Analyzer, "p")
}

func TestAnalyzerSkipsGeneratedFiles(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), pure.Analyzer, "g")
}
