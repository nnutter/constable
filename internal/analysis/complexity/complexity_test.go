package complexity_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/complexity"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), complexity.Analyzer, "c")
}
