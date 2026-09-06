package testify_test

import (
	"testing"

	"github.com/nnutter/constable/internal/analysis/testify"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), testify.Analyzer, "t")
}
