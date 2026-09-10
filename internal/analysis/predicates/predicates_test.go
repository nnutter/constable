package predicates_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/predicates"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), predicates.Analyzer, "p")
}
