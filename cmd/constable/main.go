package main

import (
	"github.com/nnutter/constable/internal/analysis/nonmutating"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(nonmutating.Analyzer)
}
