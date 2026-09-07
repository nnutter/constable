package main

import (
	"github.com/nnutter/constable/internal/analysis/methodical"
	"github.com/nnutter/constable/internal/analysis/nonmutating"
	"github.com/nnutter/constable/internal/analysis/testify"
	"github.com/nnutter/constable/internal/driver"
)

func main() {
	driver.Main(nonmutating.Analyzer, methodical.Analyzer, testify.Analyzer)
}
