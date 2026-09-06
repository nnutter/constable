package methodical_test

import (
	"testing"

	"github.com/nnutter/constable/internal/analysis/methodical"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), methodical.Analyzer, "m")
}
