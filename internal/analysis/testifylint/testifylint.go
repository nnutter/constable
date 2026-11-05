// Package testifylint adapts the testifylint analyzer to constable.
//
// constable embeds the analyzer from another module instead of reimplementing
// it, so this package holds the places where constable's own policy differs
// from the upstream default.
package testifylint

import (
	"github.com/nnutter/testifylint/analyzer"

	"github.com/nnutter/constable/internal/generated"
)

// Analyzer reports tests that misuse testify. It is the upstream analyzer with
// constable's generated-file policy applied.
var Analyzer = generated.Skip(analyzer.New())
