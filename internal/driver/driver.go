// Package driver is a multichecker-compatible analysis driver that
// prints file paths relative to the current working directory.
package driver

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/analysis/unitchecker"
	"golang.org/x/tools/go/packages"
)

// Options controls driver output.
type Options struct {
	JSON         bool
	Context      int
	IncludeTests bool
	Fix          bool
}

type triState int

const (
	unset triState = iota
	setTrue
	setFalse
)

func (t *triState) IsBoolFlag() bool { return true }

func (t *triState) Set(s string) error {
	switch s {
	case "true":
		*t = setTrue
	case "false":
		*t = setFalse
	default:
		return fmt.Errorf("invalid value %q: must be true or false", s)
	}
	return nil
}

func (t *triState) String() string {
	switch *t {
	case setTrue:
		return "true"
	case setFalse:
		return "false"
	default:
		return ""
	}
}

// Main is the main function for the constable command.
// version is the release version injected at build time via
// -ldflags "-X main.version=..."; when empty, the embedded build info is used.
func Main(version string, analyzers ...*analysis.Analyzer) {
	progname := filepath.Base(os.Args[0])
	log.SetFlags(0)
	log.SetPrefix(progname + ": ")

	if err := analysis.Validate(analyzers); err != nil {
		log.Fatal(err)
	}

	var opts Options
	opts.IncludeTests = true
	opts.Context = -1

	enabled := make(map[*analysis.Analyzer]*triState)
	for _, a := range analyzers {
		enable := new(triState)
		flag.Var(enable, a.Name, fmt.Sprintf("enable %q analysis", a.Name))
		enabled[a] = enable

		a.Flags.VisitAll(func(f *flag.Flag) {
			name := a.Name + "." + f.Name
			if flag.Lookup(name) == nil {
				flag.Var(f.Value, name, f.Usage)
			}
		})
	}

	printFlags := flag.Bool("flags", false, "print analyzer flags in JSON")
	showVersion := flag.Bool("V", false, "print version and exit")
	flag.BoolVar(&opts.JSON, "json", false, "emit JSON output")
	flag.IntVar(&opts.Context, "c", -1, "display offending line with this many lines of context")
	flag.BoolVar(&opts.Fix, "fix", false, "apply all suggested fixes")
	flag.BoolVar(&diff, "diff", false, "with -fix, don't update the files, but print a unified diff")
	flag.BoolVar(&opts.IncludeTests, "test", true, "indicates whether test files should be analyzed, too")

	flag.Parse()

	if *printFlags {
		writeFlagsJSON()
		return
	}

	if *showVersion {
		writeVersion(progname, version)
		return
	}

	analyzers = filterAnalyzers(analyzers, enabled)

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "%[1]s is a tool for static analysis of Go programs.\n\nUsage: %[1]s [-flag] [package]\n\nRun '%[1]s help' for more detail,\n or '%[1]s help name' for details and flags of a specific analyzer.\n", progname)
		os.Exit(1)
	}

	if args[0] == "help" {
		writeHelp(progname, analyzers, args[1:])
		return
	}

	if hasSingleArg(args) && isConfig(args[0]) {
		unitchecker.Run(args[0], analyzers)
		panic("unreachable")
	}

	os.Exit(Run(args, analyzers, opts))
}

var diff bool // accepted for multichecker compatibility; no analyzers emit fixes.

func succeeded(ok bool) bool {
	return ok
}

func hasSingleArg(args []string) bool {
	return len(args) == 1
}

func isConfig(path string) bool {
	return strings.HasSuffix(path, ".cfg")
}

func hasEnabled(enabled map[*analysis.Analyzer]*triState) bool {
	return enabled != nil
}

func hasAnalyzer(enabled map[*analysis.Analyzer]*triState, a *analysis.Analyzer) bool {
	if enabled == nil {
		return false
	}
	return enabled[a] != nil
}

func isNotDisabledFor(enabled map[*analysis.Analyzer]*triState, a *analysis.Analyzer) bool {
	if enabled == nil {
		return false
	}
	state := enabled[a]
	if state == nil {
		return false
	}
	return *state != setFalse
}

func isEnabledFor(enabled map[*analysis.Analyzer]*triState, a *analysis.Analyzer) bool {
	if enabled == nil {
		return false
	}
	state := enabled[a]
	if state == nil {
		return false
	}
	return *state == setTrue
}

func isBoolFlagged(ok bool) bool {
	return ok
}

func hasVersion(info *debug.BuildInfo) bool {
	if info == nil {
		return false
	}
	return info.Main.Version != ""
}

func isNotDevel(info *debug.BuildInfo) bool {
	if info == nil {
		return false
	}
	return info.Main.Version != "(devel)"
}

func versionOf(info *debug.BuildInfo) string {
	if info == nil {
		return ""
	}
	return info.Main.Version
}

func isNilErr(err error) bool {
	return err == nil
}

func isEmptyInitial(initial []*packages.Package) bool {
	return len(initial) == 0
}

func isFirstLineOrLater(i int) bool {
	return 1 <= i
}

func isWithinLines(i int, lines []string) bool {
	return i <= len(lines)
}

func isRoot(act *checker.Action) bool {
	return act.IsRoot
}

func hasDiagnostics(diagnostics []analysis.Diagnostic) bool {
	return len(diagnostics) > 0
}

// filterAnalyzers mirrors multichecker's -name enable-flag semantics.
func filterAnalyzers(analyzers []*analysis.Analyzer, enabled map[*analysis.Analyzer]*triState) []*analysis.Analyzer {
	var hasTrue, hasFalse bool
	for _, ts := range enabled {
		switch *ts {
		case setTrue:
			hasTrue = true
		case setFalse:
			hasFalse = true
		}
	}

	if hasTrue {
		return keepEnabled(analyzers, enabled)
	}
	if hasFalse {
		var keep []*analysis.Analyzer
		for _, a := range analyzers {
			if hasEnabled(enabled) && hasAnalyzer(enabled, a) && isNotDisabledFor(enabled, a) {
				keep = append(keep, a)
			}
		}
		return keep
	}
	return analyzers
}

func keepEnabled(analyzers []*analysis.Analyzer, enabled map[*analysis.Analyzer]*triState) []*analysis.Analyzer {
	var keep []*analysis.Analyzer
	for _, a := range analyzers {
		if hasEnabled(enabled) && hasAnalyzer(enabled, a) && isEnabledFor(enabled, a) {
			keep = append(keep, a)
		}
	}
	return keep
}

type jsonFlag struct {
	Name  string `json:"Name"`
	Bool  bool   `json:"Bool"`
	Usage string `json:"Usage"`
}

func writeFlagsJSON() {
	var flags []jsonFlag
	flag.VisitAll(func(f *flag.Flag) {
		if f.Name == "fix" {
			return
		}
		b, ok := f.Value.(interface{ IsBoolFlag() bool })
		flags = append(flags, jsonFlag{f.Name, isBoolFlagged(ok) && b.IsBoolFlag(), f.Usage})
	})
	data, err := json.MarshalIndent(flags, "", "\t")
	if err != nil {
		log.Fatal(err)
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s", data)
}

func writeVersion(progname, version string) {
	_, _ = fmt.Fprintf(os.Stdout, "%s %s\n", progname, resolveVersion(version))
}

// resolveVersion prefers the injected version and falls back to the
// embedded build info. The "(devel)" placeholder means the binary was
// built without module version or VCS info (e.g. from a tarball), so it
// is treated as unset.
func resolveVersion(version string) string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); succeeded(ok) && hasVersion(info) && isNotDevel(info) {
		return versionOf(info)
	}
	return "devel"
}

func writeHelp(progname string, analyzers []*analysis.Analyzer, args []string) {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "%s is a tool for static analysis of Go programs.\n\nAnalyzers:\n\n", progname)
		for _, a := range analyzers {
			paras := strings.Split(a.Doc, "\n\n")
			_, _ = fmt.Fprintf(os.Stdout, "%s: %s\n\n", a.Name, paras[0])
		}
		_, _ = fmt.Fprintf(os.Stdout, "Run '%s help name' for details and flags of a specific analyzer.\n", progname)
		return
	}
	for _, a := range analyzers {
		if a.Name == args[0] {
			_, _ = fmt.Fprintf(os.Stdout, "%s: %s\n", a.Name, a.Doc)
			a.Flags.VisitAll(func(f *flag.Flag) {
				_, _ = fmt.Fprintf(os.Stdout, "-%s.%s: %s\n", a.Name, f.Name, f.Usage)
			})
			return
		}
	}
	fmt.Fprintf(os.Stderr, "unknown analyzer %q\n", args[0])
	os.Exit(1)
}

// Run loads the packages and applies the analyzers, printing
// diagnostics with paths relative to the current working directory.
// It returns the appropriate exit code.
func Run(args []string, analyzers []*analysis.Analyzer, opts Options) (exitcode int) {
	exitAtLeast := func(code int) {
		exitcode = max(code, exitcode)
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Print(err)
		return 1
	}

	initial, err := load(args, opts.IncludeTests, needFacts(analyzers))
	if err != nil {
		log.Print(err)
		exitAtLeast(1)
		return exitcode
	}

	if n := packages.PrintErrors(initial); n > 0 {
		exitAtLeast(1)
	}

	graph, err := checker.Analyze(analyzers, initial, &checker.Options{})
	if err != nil {
		log.Print(err)
		exitAtLeast(1)
		return exitcode
	}

	if opts.Fix {
		return exitcode
	}

	// With -json, the exit code is always zero.
	if opts.JSON {
		if err := printJSON(os.Stdout, graph, cwd); err != nil {
			return 1
		}
		return exitcode
	}

	if err := printText(os.Stderr, graph, cwd, opts.Context); err != nil {
		return 1
	}

	exitAtLeast(exitCode(graph))

	return exitcode
}

func exitCode(graph *checker.Graph) int {
	var numErrors, rootDiags int
	for act := range graph.All() {
		if act.Err != nil {
			numErrors++
		} else if act.IsRoot {
			rootDiags += len(act.Diagnostics)
		}
	}

	if numErrors > 0 {
		return 1
	} else if rootDiags > 0 {
		return 3
	}

	return 0
}

func needFacts(analyzers []*analysis.Analyzer) bool {
	seen := make(map[*analysis.Analyzer]bool)
	var queue []*analysis.Analyzer
	queue = append(queue, analyzers...)
	for len(queue) > 0 {
		a := queue[0]
		queue = queue[1:]
		if !seen[a] {
			seen[a] = true
			if len(a.FactTypes) > 0 {
				return true
			}
			queue = append(queue, a.Requires...)
		}
	}
	return false
}

func load(patterns []string, includeTests, allSyntax bool) ([]*packages.Package, error) {
	mode := packages.LoadSyntax
	if allSyntax {
		mode = packages.LoadAllSyntax
	}
	mode |= packages.NeedModule
	conf := packages.Config{
		Mode:  mode,
		Tests: includeTests,
	}
	initial, err := packages.Load(&conf, patterns...)
	if isNilErr(err) && isEmptyInitial(initial) {
		err = fmt.Errorf("%s matched no packages", strings.Join(patterns, " "))
	}
	return initial, err
}

// relativePath returns filename relative to cwd, or filename unchanged
// when relativizing fails.
func relativePath(cwd, filename string) string {
	if filename == "" {
		return filename
	}
	if rel, err := filepath.Rel(cwd, filename); err == nil {
		return rel
	}
	return filename
}

func RelativePosition(cwd string, fset *token.FileSet, pos token.Pos) token.Position {
	position := fset.Position(pos)
	position.Filename = relativePath(cwd, position.Filename)
	return position
}

func printText(w *os.File, graph *checker.Graph, cwd string, contextLines int) error {
	type key struct {
		pos     token.Position
		end     token.Position
		message string
		name    string
	}
	seen := make(map[key]bool)

	for act := range graph.All() {
		if act.Err != nil {
			_, _ = fmt.Fprintf(w, "%s: %v\n", act.Analyzer.Name, act.Err)
		} else if act.IsRoot {
			for _, diag := range act.Diagnostics {
				posn := act.Package.Fset.Position(diag.Pos)
				end := act.Package.Fset.Position(diag.End)
				k := key{posn, end, diag.Message, act.Analyzer.Name}
				if seen[k] {
					continue
				}
				seen[k] = true

				printDiagnostic(w, act.Package.Fset, cwd, contextLines, diag)
			}
		}
	}
	return nil
}

func printDiagnostic(w *os.File, fset *token.FileSet, cwd string, contextLines int, diag analysis.Diagnostic) {
	printOne := func(pos, end token.Pos, message string) {
		abs := fset.Position(pos)
		rel := abs
		rel.Filename = relativePath(cwd, abs.Filename)
		_, _ = fmt.Fprintf(w, "%s: %s\n", rel, message)

		if contextLines >= 0 {
			endPosition := fset.Position(end)
			if !endPosition.IsValid() {
				endPosition = abs
			}
			data, _ := os.ReadFile(abs.Filename)
			lines := strings.Split(string(data), "\n")
			for i := abs.Line - contextLines; i <= endPosition.Line+contextLines; i++ {
				if isFirstLineOrLater(i) && isWithinLines(i, lines) {
					_, _ = fmt.Fprintf(w, "%d\t%s\n", i, lines[i-1])
				}
			}
		}
	}

	printOne(diag.Pos, diag.End, diag.Message)
	for _, rel := range diag.Related {
		printOne(rel.Pos, rel.End, "\t"+rel.Message)
	}
}

type jsonTextEdit struct {
	Filename string `json:"filename"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	New      string `json:"new"`
}

type jsonSuggestedFix struct {
	Message string         `json:"message"`
	Edits   []jsonTextEdit `json:"edits"`
}

type jsonRelated struct {
	Posn    string `json:"posn"`
	End     string `json:"end"`
	Message string `json:"message"`
}

type jsonDiagnostic struct {
	Category       string             `json:"category,omitempty"`
	Posn           string             `json:"posn"`
	End            string             `json:"end"`
	Message        string             `json:"message"`
	SuggestedFixes []jsonSuggestedFix `json:"suggested_fixes,omitempty"`
	Related        []jsonRelated      `json:"related,omitempty"`
}

func encodeDiagnostic(diag analysis.Diagnostic, fset *token.FileSet, cwd string) jsonDiagnostic {
	var fixes []jsonSuggestedFix
	for _, fix := range diag.SuggestedFixes {
		var edits []jsonTextEdit
		for _, edit := range fix.TextEdits {
			edits = append(edits, jsonTextEdit{
				Filename: relativePath(cwd, fset.Position(edit.Pos).Filename),
				Start:    fset.Position(edit.Pos).Offset,
				End:      fset.Position(edit.End).Offset,
				New:      string(edit.NewText),
			})
		}
		fixes = append(fixes, jsonSuggestedFix{Message: fix.Message, Edits: edits})
	}
	var related []jsonRelated
	for _, r := range diag.Related {
		related = append(related, jsonRelated{
			Posn:    RelativePosition(cwd, fset, r.Pos).String(),
			End:     RelativePosition(cwd, fset, cmp.Or(r.End, r.Pos)).String(),
			Message: r.Message,
		})
	}
	return jsonDiagnostic{
		Category:       diag.Category,
		Posn:           RelativePosition(cwd, fset, diag.Pos).String(),
		End:            RelativePosition(cwd, fset, cmp.Or(diag.End, diag.Pos)).String(),
		Message:        diag.Message,
		SuggestedFixes: fixes,
		Related:        related,
	}
}

func printJSON(w *os.File, graph *checker.Graph, cwd string) error {
	tree := make(map[string]map[string]any)
	for act := range graph.All() {
		var value any
		if act.Err != nil {
			value = map[string]string{"error": act.Err.Error()}
		} else if isRoot(act) && hasDiagnostics(act.Diagnostics) {
			diagnostics := make([]jsonDiagnostic, 0, len(act.Diagnostics))
			for _, diag := range act.Diagnostics {
				diagnostics = append(diagnostics, encodeDiagnostic(diag, act.Package.Fset, cwd))
			}
			value = diagnostics
		}
		if value != nil {
			entries, ok := tree[act.Package.ID]
			if !ok {
				entries = make(map[string]any)
				tree[act.Package.ID] = entries
			}
			entries[act.Analyzer.Name] = value
		}
	}
	data, err := json.MarshalIndent(tree, "", "\t")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}
