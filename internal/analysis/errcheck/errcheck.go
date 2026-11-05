// Package errcheck adapts the errcheck analyzer to constable.
//
// constable embeds the analyzer from another module instead of reimplementing
// it, so this package holds the places where constable's own policy differs
// from the upstream default.
package errcheck

import (
	upstream "github.com/kisielk/errcheck/errcheck"

	"github.com/nnutter/constable/internal/generated"
)

// Analyzer reports unchecked errors. It is the upstream analyzer with
// constable's generated-file policy applied.
var Analyzer = generated.Skip(upstream.Analyzer)
