package methodical_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/methodical"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), methodical.Analyzer, "m")
}
