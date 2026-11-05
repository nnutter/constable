// Package nilaway adapts the nilaway analyzer to constable.
//
// constable embeds the analyzer from another module instead of reimplementing
// it, so this package holds the places where constable's own policy differs
// from the upstream default.
package nilaway

import (
	"flag"
	"sort"
	"strings"

	"go.uber.org/nilaway"
	nilawayconfig "go.uber.org/nilaway/config"
	"golang.org/x/tools/go/packages"

	"github.com/nnutter/constable/internal/generated"
)

// Analyzer reports a nil value that flows to a dereference. It is the upstream
// analyzer with constable's generated-file policy applied and with the
// analyzed packages limited to the module that constable runs on.
var Analyzer = generated.Skip(nilaway.Analyzer)

// includeAllPkgs keeps nilaway's own default, every package that it loads.
var includeAllPkgs bool

func init() {
	Analyzer.Flags.BoolVar(&includeAllPkgs, "include-all-pkgs", false,
		"analyze every package that nilaway loads, including dependencies and the standard library")

	// nilaway pretty-prints a multi-line explanation by default, with color
	// when the terminal supports it. constable prints one diagnostic per line
	// for editors and hooks, so the default is off. A flag still turns it on.
	if flag := nilawayconfig.Analyzer.Flags.Lookup(nilawayconfig.PrettyPrintFlag); flag != nil {
		_ = flag.Value.Set("false")
	}
}

// UseMainModules limits nilaway to the main modules of the packages that the
// driver loaded, unless the command line chose the packages itself.
//
// NilAway reads every package that it loads by default, which costs seconds
// and gigabytes and reports flows that the analyzed repository cannot fix. Its
// -include-pkgs flag takes package path prefixes, and the module path of a
// root covers every package of that module.
//
// UseMainModules is a driver.Hooks.AfterLoad step: the packages and their
// modules exist only after the driver loads them.
func UseMainModules(roots []*packages.Package) {
	if includeAllPkgs {
		return
	}

	chosen := nilawayconfig.Analyzer.Flags.Lookup(nilawayconfig.IncludePkgsFlag)
	if chosen == nil {
		return
	}
	if isChosen(chosen) {
		return
	}

	modules := mainModules(roots)
	if len(modules) == 0 {
		return
	}
	_ = chosen.Value.Set(strings.Join(modules, ","))
}

// mainModules returns the sorted paths of the main modules of roots.
//
// A root outside a module, such as a package of the standard library that a
// pattern names, contributes nothing.
func mainModules(roots []*packages.Package) []string {
	var paths []string
	seen := make(map[string]bool)
	for _, root := range roots {
		if isNilRoot(root) || isNilModule(root.Module) || isDependencyModule(root.Module) || isSeen(seen, root.Module.Path) {
			continue
		}
		seen[root.Module.Path] = true
		paths = append(paths, root.Module.Path)
	}
	sort.Strings(paths)
	return paths
}

func isChosen(f *flag.Flag) bool {
	return f.Value.String() != ""
}

func isNilRoot(root *packages.Package) bool {
	return root == nil
}

func isNilModule(module *packages.Module) bool {
	return module == nil
}

func isDependencyModule(module *packages.Module) bool {
	return !module.Main
}

func isSeen(seen map[string]bool, path string) bool {
	return seen[path]
}
