// Package nilaway adapts the nilaway analyzer to constable.
//
// constable embeds the analyzer from another module instead of reimplementing
// it, so this package holds the places where constable's own policy differs
// from the upstream default.
package nilaway

import (
	"go.uber.org/nilaway"
	nilawayconfig "go.uber.org/nilaway/config"

	"github.com/nnutter/constable/internal/generated"
)

// Analyzer reports a nil value that flows to a dereference. It is the upstream
// analyzer with constable's generated-file policy applied.
var Analyzer = generated.Skip(nilaway.Analyzer)

func init() {
	// nilaway pretty-prints a multi-line explanation by default, with color
	// when the terminal supports it. constable prints one diagnostic per line
	// for editors and hooks, so the default is off. A flag still turns it on.
	if flag := nilawayconfig.Analyzer.Flags.Lookup(nilawayconfig.PrettyPrintFlag); flag != nil {
		_ = flag.Value.Set("false")
	}
}
