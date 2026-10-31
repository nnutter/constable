package driver_test

import (
	"fmt"
	"go/ast"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/analysis/methodical"
	"github.com/nnutter/constable/internal/driver"
)

func fixRepo(t *testing.T, source string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fix\n\ngo 1.25\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fix.go"), []byte(source), 0o644))
	gitCommand(t, dir, "", "init", "-q")
	gitCommand(t, dir, "", "add", ".")
	gitCommand(t, dir, "", "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "initial")
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(cwd)) })
	return dir
}

func gitCommand(t *testing.T, dir, input string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func runFix(t *testing.T, analyzer *analysis.Analyzer, diff bool) (int, string, string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "out")
	require.NoError(t, err)
	errout, err := os.CreateTemp(t.TempDir(), "err")
	require.NoError(t, err)
	oldout, olderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, errout
	code := driver.Run([]string{"."}, []*analysis.Analyzer{analyzer}, driver.Options{Fix: true, Diff: diff, Context: -1})
	os.Stdout, os.Stderr = oldout, olderr
	require.NoError(t, out.Close())
	require.NoError(t, errout.Close())
	output, err := os.ReadFile(out.Name())
	require.NoError(t, err)
	diagnostics, err := os.ReadFile(errout.Name())
	require.NoError(t, err)
	return code, string(output), string(diagnostics)
}

func TestMethodicalCumulativeFix(t *testing.T) {
	source := "package fix\n\ntype First struct{}\n\nfunc (First) Z() {}\n\nfunc (First) A() {}\n\ntype Second struct{}\n\nfunc (Second) Z() {}\n\nfunc (Second) A() {}\n"
	dir := fixRepo(t, source)
	code, diff, diagnostics := runFix(t, methodical.Analyzer, true)
	require.Equal(t, 0, code)
	require.Empty(t, diagnostics)
	require.NotEmpty(t, diff)
	original, err := os.ReadFile("fix.go")
	require.NoError(t, err)
	require.Equal(t, source, string(original))
	gitCommand(t, dir, diff, "apply", "-p0", "-")
	fromDiff, err := os.ReadFile("fix.go")
	require.NoError(t, err)
	gitCommand(t, dir, "", "checkout", "--", "fix.go")
	code, output, diagnostics := runFix(t, methodical.Analyzer, false)
	require.Equal(t, 0, code)
	require.Empty(t, output)
	require.Empty(t, diagnostics)
	applied, err := os.ReadFile("fix.go")
	require.NoError(t, err)
	require.Equal(t, fromDiff, applied)
	require.Less(t, strings.Index(string(applied), "func (First) A"), strings.Index(string(applied), "func (First) Z"))
	require.Less(t, strings.Index(string(applied), "func (Second) A"), strings.Index(string(applied), "func (Second) Z"))
	for _, diffMode := range []bool{false, true} {
		code, output, diagnostics = runFix(t, methodical.Analyzer, diffMode)
		require.Equal(t, 0, code)
		require.Empty(t, output)
		require.Empty(t, diagnostics)
		again, err := os.ReadFile("fix.go")
		require.NoError(t, err)
		require.Equal(t, applied, again)
	}
}

func TestFixMoreThan100Passes(t *testing.T) {
	source := "package fix\nvar Count = 0\n"
	fixRepo(t, source)
	analyzer := &analysis.Analyzer{Name: "increment", Doc: "increment the count", Run: func(pass *analysis.Pass) (any, error) {
		for _, file := range pass.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok {
					return true
				}
				value, err := strconv.Atoi(literal.Value)
				if err != nil {
					return true
				}
				if value < 101 {
					pass.Report(analysis.Diagnostic{Pos: literal.Pos(), Message: "increment", SuggestedFixes: []analysis.SuggestedFix{{TextEdits: []analysis.TextEdit{{Pos: literal.Pos(), End: literal.End(), NewText: []byte(strconv.Itoa(value + 1))}}}}})
				}
				return true
			})
		}
		return nil, nil
	}}
	code, output, diagnostics := runFix(t, analyzer, true)
	require.Equal(t, 0, code)
	require.Empty(t, diagnostics)
	require.Contains(t, output, "-var Count = 0\n")
	require.Contains(t, output, "+var Count = 101\n")
	data, err := os.ReadFile("fix.go")
	require.NoError(t, err)
	require.Equal(t, source, string(data))
}

func TestLaterAnalysisFailureDoesNotWrite(t *testing.T) {
	source := "package fix\nvar Alpha = 1\n"
	fixRepo(t, source)
	analyzer := &analysis.Analyzer{Name: "laterfailure", Doc: "fails after an edit", Run: func(pass *analysis.Pass) (any, error) {
		for _, file := range pass.Files {
			var failed bool
			ast.Inspect(file, func(node ast.Node) bool {
				id, ok := node.(*ast.Ident)
				if !ok {
					return true
				}
				if id.Name == "Bravo" {
					failed = true
				}
				if id.Name == "Alpha" {
					pass.Report(analysis.Diagnostic{Pos: id.Pos(), Message: "change Alpha", SuggestedFixes: []analysis.SuggestedFix{{TextEdits: []analysis.TextEdit{{Pos: id.Pos(), End: id.End(), NewText: []byte("Bravo")}}}}})
				}
				return true
			})
			if failed {
				return nil, fmt.Errorf("later analysis failed")
			}
		}
		return nil, nil
	}}
	for _, diff := range []bool{false, true} {
		code, output, diagnostics := runFix(t, analyzer, diff)
		require.Equal(t, 1, code)
		require.Empty(t, output)
		require.Contains(t, diagnostics, "later analysis failed")
		data, err := os.ReadFile("fix.go")
		require.NoError(t, err)
		require.Equal(t, source, string(data))
	}
}

func TestFixCycleDoesNotWrite(t *testing.T) {
	source := "package fix\nvar Alpha = 1\n"
	fixRepo(t, source)
	analyzer := &analysis.Analyzer{Name: "cycle", Doc: "alternates identifiers", Run: func(pass *analysis.Pass) (any, error) {
		for _, file := range pass.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				id, ok := node.(*ast.Ident)
				if !ok {
					return true
				}
				var next string
				switch id.Name {
				case "Alpha":
					next = "Bravo"
				case "Bravo":
					next = "Alpha"
				default:
					return true
				}
				pass.Report(analysis.Diagnostic{Pos: id.Pos(), Message: fmt.Sprintf("change %s", id.Name), SuggestedFixes: []analysis.SuggestedFix{{TextEdits: []analysis.TextEdit{{Pos: id.Pos(), End: id.End(), NewText: []byte(next)}}}}})
				return true
			})
		}
		return nil, nil
	}}
	for _, diff := range []bool{false, true} {
		code, output, diagnostics := runFix(t, analyzer, diff)
		require.Equal(t, 1, code)
		require.Empty(t, output)
		require.Contains(t, diagnostics, "change Bravo")
		data, err := os.ReadFile("fix.go")
		require.NoError(t, err)
		require.Equal(t, source, string(data))
	}
}
