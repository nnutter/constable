package sumtype_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/sumtype"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), sumtype.Analyzer, "st")
}
