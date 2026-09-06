package main

import (
	"github.com/nnutter/constable/internal/analysis/methodical"
	"github.com/nnutter/constable/internal/analysis/nonmutating"
	"github.com/nnutter/constable/internal/analysis/testify"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(nonmutating.Analyzer, methodical.Analyzer, testify.Analyzer)
}
