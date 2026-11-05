// Package sumtype adapts the go-check-sumtype analyzer to constable.
//
// constable embeds the analyzer from another module instead of reimplementing
// it, so this package holds the places where constable's own policy differs
// from the upstream default.
package sumtype

import (
	gochecksumtype "github.com/alecthomas/go-check-sumtype"

	"github.com/nnutter/constable/internal/generated"
)

// Analyzer reports a type switch over a sum type that omits a variant. It is
// the upstream analyzer with constable's generated-file policy applied.
var Analyzer = generated.Skip(gochecksumtype.Analyzer)
