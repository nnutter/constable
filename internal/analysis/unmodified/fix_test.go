package unmodified_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/unmodified"
)

type capturedDiagnostic struct {
	message string
	edits   int
}

func inspectUnmodified(t *testing.T, dir string) []capturedDiagnostic {
	t.Helper()
	var diagnostics []capturedDiagnostic
	analyzer := &analysis.Analyzer{
		Name: "captureunmodified",
		Doc:  "capture unmodified diagnostics",
		Run: func(pass *analysis.Pass) (any, error) {
			pass.Report = func(d analysis.Diagnostic) {
				edits := 0
				for _, fix := range d.SuggestedFixes {
					edits += len(fix.TextEdits)
				}
				diagnostics = append(diagnostics, capturedDiagnostic{message: d.Message, edits: edits})
			}
			return unmodified.Analyzer.Run(pass)
		},
	}
	analysistest.Run(t, dir, analyzer, "p")
	return diagnostics
}

func writeUnmodifiedFixture(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, "src", "p", "p.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func findCaptured(t *testing.T, diagnostics []capturedDiagnostic, name string) capturedDiagnostic {
	t.Helper()
	for _, d := range diagnostics {
		if strings.Contains(d.message, "variable "+name+" ") {
			return d
		}
	}
	require.Fail(t, "missing diagnostic for "+name, "%+v", diagnostics)
	return capturedDiagnostic{}
}

func TestFixSkipsPartialSpec(t *testing.T) {
	dir := t.TempDir()
	writeUnmodifiedFixture(t, dir, "package p\n\nvar kept, changed = 1, 2\n\nfunc Mutate() { changed = 3 }\n\nfunc Use() int { return kept + changed }\n")
	diagnostics := inspectUnmodified(t, dir)
	require.Empty(t, findCaptured(t, diagnostics, "kept").edits, "partial spec offered a whole-spec fix")
}

func TestFixSkipsPartialGroup(t *testing.T) {
	dir := t.TempDir()
	writeUnmodifiedFixture(t, dir, "package p\n\nvar (\n\tkept = 1\n\tchanged = 2\n)\n\nfunc Mutate() { changed = 3 }\n\nfunc Use() int { return kept + changed }\n")
	diagnostics := inspectUnmodified(t, dir)
	require.Empty(t, findCaptured(t, diagnostics, "kept").edits, "partial group offered a whole-group fix")
}

func TestFixSkipsSuppressedVariable(t *testing.T) {
	dir := t.TempDir()
	writeUnmodifiedFixture(t, dir, "package p\n\n//nolint:unmodified\nvar suppressed = 1\n\nvar reported = 2\n\nfunc Use() int { return suppressed + reported }\n")
	diagnostics := inspectUnmodified(t, dir)
	for _, d := range diagnostics {
		require.NotContains(t, d.message, "variable suppressed ")
	}
	require.NotEmpty(t, findCaptured(t, diagnostics, "reported").edits, "unsuppressed variable offered no fix")
}

func TestFixOffersCompleteGroup(t *testing.T) {
	dir := t.TempDir()
	writeUnmodifiedFixture(t, dir, "package p\n\nvar (\n\tfirst = 1\n\tsecond = 2\n)\n\nfunc Use() int { return first + second }\n")
	diagnostics := inspectUnmodified(t, dir)
	require.NotEmpty(t, findCaptured(t, diagnostics, "first").edits, "complete group offered no fix")
	require.NotEmpty(t, findCaptured(t, diagnostics, "second").edits, "complete group offered no fix")
}
