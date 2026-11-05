package main

import (
	"github.com/nnutter/constable/internal/analysis/complexity"
	"github.com/nnutter/constable/internal/analysis/errcheck"
	"github.com/nnutter/constable/internal/analysis/methodical"
	"github.com/nnutter/constable/internal/analysis/nilaway"
	"github.com/nnutter/constable/internal/analysis/nonmutating"
	"github.com/nnutter/constable/internal/analysis/predicates"
	"github.com/nnutter/constable/internal/analysis/sumtype"
	"github.com/nnutter/constable/internal/analysis/testify"
	"github.com/nnutter/constable/internal/analysis/testifylint"
	"github.com/nnutter/constable/internal/driver"
)

// version is the release version, injected at build time via
// -ldflags "-X main.version=...". It defaults to empty so resolveVersion
// falls back to the embedded build info.
var version string

func main() {
	driver.Main(
		version,
		driver.Hooks{},
		nonmutating.Analyzer,
		methodical.Analyzer,
		predicates.Analyzer,
		testify.Analyzer,
		complexity.Analyzer,
		errcheck.Analyzer,
		sumtype.Analyzer,
		testifylint.Analyzer,
		nilaway.Analyzer,
	)
}
