package main

import (
	"github.com/nnutter/constable/internal/analysis/complexity"
	"github.com/nnutter/constable/internal/analysis/methodical"
	"github.com/nnutter/constable/internal/analysis/nonmutating"
	"github.com/nnutter/constable/internal/analysis/testify"
	"github.com/nnutter/constable/internal/driver"
)

// version is the release version, injected at build time via
// -ldflags "-X main.version=...". It defaults to empty so resolveVersion
// falls back to the embedded build info.
var version string

func main() {
	driver.Main(version, nonmutating.Analyzer, methodical.Analyzer, testify.Analyzer, complexity.Analyzer)
}
