package nilaway_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/nilaway"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), nilaway.Analyzer, "na")
}
